//go:build linux

// Package vklogin opens a native WebKitGTK window for VK account sign-in and
// captures the resulting web-session cookies. The app's own webview (served over
// the wails:// custom scheme) cannot read HttpOnly cookies like remixsid, so this
// runs a dedicated GTK4 + webkitgtk-6.0 window with its own persistent network
// session and reads the cookie jar directly via WebKitCookieManager. The captured
// "k=v; k=v" cookie header plus the browser User-Agent are what the relay needs to
// mint a privileged VK TURN token (account mode / B').
package vklogin

/*
#cgo pkg-config: gtk4 webkitgtk-6.0 libsoup-3.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <libsoup/soup.h>
#include <stdlib.h>
#include <string.h>

extern void vkloginResult(char* cookies, char* ua, char* err);

typedef struct {
    char* url;
    char* storage_dir;
    char* user_agent;
    GtkWidget* window;
    WebKitWebView* webview;
    WebKitCookieManager* cm;
    int finished;
} vk_ctx;

// finish delivers the result exactly once and closes the window. It deliberately
// does NOT free ctx: WebKit/GTK can still fire queued signals (load-changed, the
// async cookie callback) on this ctx while the window tears down, and a freed ctx
// would be a use-after-free (segfault in the GTK main loop). ctx is a few hundred
// bytes and sign-in is rare, so it is left to leak rather than freed unsafely.
static gboolean destroy_window_idle(gpointer window) {
    gtk_window_destroy(GTK_WINDOW(window));
    return G_SOURCE_REMOVE;
}

static void finish(vk_ctx* c, const char* cookies, const char* ua, const char* err) {
    if (c->finished) return;
    c->finished = 1;
    vkloginResult((char*)cookies, (char*)ua, (char*)err);
    // Destroy on the next main-loop turn, not synchronously: finish() runs inside a
    // WebKit cookie callback, and tearing the webview down from within its own
    // subsystem's callback is reentrant and crashes.
    g_idle_add(destroy_window_idle, c->window);
}

// on_cookies_ready fires on the GTK main loop with the current cookie jar for
// vk.com. It only completes once remixsid is present (the signed-in signal).
static void on_cookies_ready(GObject* src, GAsyncResult* res, gpointer data) {
    vk_ctx* c = (vk_ctx*)data;
    GError* err = NULL;
    GList* cookies = webkit_cookie_manager_get_cookies_finish(WEBKIT_COOKIE_MANAGER(src), res, &err);
    if (err != NULL) {
        g_error_free(err);
        if (cookies) g_list_free_full(cookies, (GDestroyNotify)soup_cookie_free);
        return;
    }
    if (c->finished) {
        g_list_free_full(cookies, (GDestroyNotify)soup_cookie_free);
        return;
    }
    GString* jar = g_string_new(NULL);
    for (GList* l = cookies; l != NULL; l = l->next) {
        SoupCookie* ck = (SoupCookie*)l->data;
        const char* name = soup_cookie_get_name(ck);
        const char* value = soup_cookie_get_value(ck);
        if (name == NULL) continue;
        if (jar->len > 0) g_string_append(jar, "; ");
        g_string_append(jar, name);
        g_string_append_c(jar, '=');
        g_string_append(jar, value ? value : "");
    }
    g_list_free_full(cookies, (GDestroyNotify)soup_cookie_free);

    if (strstr(jar->str, "remixsid") != NULL) {
        WebKitSettings* settings = webkit_web_view_get_settings(c->webview);
        const char* ua = webkit_settings_get_user_agent(settings);
        finish(c, jar->str, ua ? ua : "", NULL);
    }
    g_string_free(jar, TRUE);
}

static void on_load_changed(WebKitWebView* wv, WebKitLoadEvent ev, gpointer data) {
    vk_ctx* c = (vk_ctx*)data;
    if (c->finished || ev != WEBKIT_LOAD_FINISHED) return;
    // Query login.vk.com: the relay mints the privileged web token against that host,
    // so its cookie set (the .vk.com session cookies plus any login.vk.com ones) is
    // exactly what the token endpoint needs.
    webkit_cookie_manager_get_cookies(c->cm, "https://login.vk.com/", NULL, on_cookies_ready, c);
}

// on_close fires when the user closes the window before signing in.
static gboolean on_close(GtkWindow* w, gpointer data) {
    finish((vk_ctx*)data, NULL, NULL, "sign-in window closed");
    return TRUE; // finish() performs the destroy
}

// create_login_window runs on the GTK main loop (scheduled via g_idle_add), where
// every GTK/WebKit call must happen.
static gboolean create_login_window(gpointer data) {
    vk_ctx* c = (vk_ctx*)data;

    WebKitNetworkSession* session = webkit_network_session_new(c->storage_dir, c->storage_dir);
    c->cm = webkit_network_session_get_cookie_manager(session);
    char* cookie_file = g_build_filename(c->storage_dir, "cookies.sqlite", NULL);
    webkit_cookie_manager_set_persistent_storage(c->cm, cookie_file, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
    g_free(cookie_file);

    c->webview = WEBKIT_WEB_VIEW(g_object_new(WEBKIT_TYPE_WEB_VIEW, "network-session", session, NULL));
    g_object_unref(session); // the webview keeps its own ref
    // Present the UA of the selected browser fingerprint so the VK web session is
    // created with the same identity the relay later impersonates. Empty = keep the
    // WebKitGTK default (a Safari UA, which VK trusts).
    if (c->user_agent && c->user_agent[0]) {
        webkit_settings_set_user_agent(webkit_web_view_get_settings(c->webview), c->user_agent);
    }

    c->window = gtk_window_new();
    gtk_window_set_title(GTK_WINDOW(c->window), "Вход в VK");
    gtk_window_set_default_size(GTK_WINDOW(c->window), 480, 720);
    gtk_window_set_child(GTK_WINDOW(c->window), GTK_WIDGET(c->webview));

    g_signal_connect(c->webview, "load-changed", G_CALLBACK(on_load_changed), c);
    g_signal_connect(c->window, "close-request", G_CALLBACK(on_close), c);

    webkit_web_view_load_uri(c->webview, c->url);
    gtk_window_present(GTK_WINDOW(c->window));
    return G_SOURCE_REMOVE;
}

// vklogin_start schedules the login window on the GTK main loop. Safe to call from
// any thread: g_idle_add hands the work to the default main context GTK runs on.
static void vklogin_start(const char* url, const char* storage_dir, const char* user_agent) {
    vk_ctx* c = g_new0(vk_ctx, 1);
    c->url = g_strdup(url);
    c->storage_dir = g_strdup(storage_dir);
    c->user_agent = g_strdup(user_agent);
    g_idle_add(create_login_window, c);
}
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

type result struct {
	cookies string
	ua      string
	err     string
}

// Only one login window runs at a time; loginMu serializes Capture calls so the
// single result channel needs no per-call token bookkeeping.
var (
	loginMu  sync.Mutex
	resultCh chan result
)

//export vkloginResult
func vkloginResult(cookies, ua, errStr *C.char) {
	r := result{}
	if cookies != nil {
		r.cookies = C.GoString(cookies)
	}
	if ua != nil {
		r.ua = C.GoString(ua)
	}
	if errStr != nil {
		r.err = C.GoString(errStr)
	}
	if resultCh != nil {
		select {
		case resultCh <- r:
		default:
		}
	}
}

// Capture opens the VK sign-in window at loginURL and blocks until the user signs
// in (remixsid appears) or closes the window. storageDir holds the persistent
// cookie store so the session survives across runs and can be re-seeded. userAgent,
// when non-empty, is presented to VK (matching the selected browser fingerprint) and
// returned back. It returns the "k=v; k=v" cookie header and the browser User-Agent.
func Capture(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
	if loginURL == "" {
		loginURL = "https://vk.com/"
	}
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		return "", "", err
	}

	loginMu.Lock()
	defer loginMu.Unlock()
	resultCh = make(chan result, 1)
	defer func() { resultCh = nil }()

	cURL := C.CString(loginURL)
	cDir := C.CString(storageDir)
	cUA := C.CString(userAgent)
	defer C.free(unsafe.Pointer(cURL))
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cUA))
	C.vklogin_start(cURL, cDir, cUA)

	r := <-resultCh
	if r.err != "" {
		return "", "", errors.New(r.err)
	}
	return r.cookies, r.ua, nil
}

// ClearStore deletes the persistent cookie store so a subsequent sign-in starts
// from a clean, logged-out VK session (the sqlite file plus its WAL/SHM sidecars).
func ClearStore(storageDir string) error {
	base := filepath.Join(storageDir, "cookies.sqlite")
	var firstErr error
	for _, p := range []string{base, base + "-wal", base + "-shm", base + "-journal"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
