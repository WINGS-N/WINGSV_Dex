//go:build windows

// Windows VK sign-in window via WebView2. The app's own webview cannot read HttpOnly
// cookies like remixsid, so this opens a dedicated WebView2 window and captures the VK
// web session by reading the "Cookie" (and "User-Agent") header off the browser's own
// outgoing requests to vk.com - those carry the full HttpOnly cookie set the relay needs.
//
// It uses the SAME WebView2 binding the Wails app uses (github.com/wailsapp/wails/webview2)
// rather than a third-party one: an independent binding's CreateCoreWebView2Environment
// call was rejected with E_ACCESSDENIED (0x80070005) on the test machine while Wails' own
// binding worked, so reusing Wails' proven path is the reliable choice. Runs OUT OF PROCESS
// (see Login / main's --vk-login hook): a process hosts one WebView2 environment and the
// app already owns one.
package vklogin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procUnregisterClassW = user32.NewProc("UnregisterClassW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
)

const (
	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	cwUseDefault       = 0x80000000
	wmSize             = 0x0005
	wmClose            = 0x0010
	wmDestroy          = 0x0002
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type win32msg struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// Capture opens the VK sign-in window and returns the captured "k=v; k=v" cookie header
// and the browser User-Agent once the VK session (remixsid) is present. It runs in the
// dedicated --vk-login child process (see Login / RunChild).
func Capture(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
	if loginURL == "" {
		loginURL = "https://vk.com/"
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE (already initialised) is fine; only a hard failure matters.
	}
	defer windows.CoUninitialize()

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className := windows.StringToUTF16Ptr("WingsvVKLoginWindow")
	title := windows.StringToUTF16Ptr("VK")

	var chromium *edge.Chromium
	var vkCookies, loginCookies, gotUA string
	loginNavDone := false

	wndProc := syscall.NewCallback(func(hwnd windows.Handle, m uint32, wParam, lParam uintptr) uintptr {
		switch m {
		case wmSize:
			if chromium != nil {
				chromium.Resize()
			}
			return 0
		case wmClose:
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		case wmDestroy:
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(m), wParam, lParam)
		return r
	})

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     windows.Handle(hInstance),
		lpszClassName: className,
	}
	// Drop any leftover registration from a prior sign-in in this process, then register.
	// The window class lives for the process lifetime, so a second Login would otherwise
	// fail here; unregister again on the way out.
	procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return "", "", errors.New("vklogin: register window class failed")
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		cwUseDefault, cwUseDefault, 980, 760,
		0, 0, uintptr(hInstance), 0,
	)
	if hwnd == 0 {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return "", "", errors.New("vklogin: create window failed")
	}
	// Tear the window down and free the class on every exit path, so a later sign-in can
	// register the class again (the cookie-capture path quits the loop without destroying
	// the window, which would otherwise keep the class alive and fail the next register).
	defer func() {
		procDestroyWindow.Call(hwnd)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
	}()

	chromium = edge.NewChromium()
	// Wails' Embed calls os.Exit(1) through its error path on a hard WebView2 failure; log
	// the real HRESULT first so the parent's captured stderr shows why (e.g. E_ACCESSDENIED).
	chromium.SetErrorCallback(func(e error) { log.Printf("vklogin: WebView2 error: %v", e) })
	dataDir := loginDataDir(storageDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Printf("vklogin: cannot create webview2 user-data folder %q: %v", dataDir, err)
	}
	chromium.DataPath = dataDir
	log.Printf("vklogin: opening WebView2 login (user-data=%q)", dataDir)
	chromium.WebResourceRequestedCallback = func(req *edge.ICoreWebView2WebResourceRequest, _ *edge.ICoreWebView2WebResourceRequestedEventArgs) {
		if loginCookies != "" {
			return
		}
		uri, _ := req.GetUri()
		if !strings.Contains(uri, "vk.com") {
			return
		}
		if u := reqHeader(req, "User-Agent"); u != "" {
			gotUA = u
		}
		cookie := reqHeader(req, "Cookie")
		if !strings.Contains(cookie, "remixsid") {
			return
		}
		// The relay mints its web token against login.vk.com, so the cookie set must be the
		// one the browser sends to THAT host (the shared .vk.com session cookies plus any
		// login.vk.com-only cookies). A plain vk.com request omits the login.vk.com-only
		// cookies and VK's token endpoint then rejects the session as unauthorized. So once
		// signed in, bounce through login.vk.com and grab the cookie header off that request;
		// keep the vk.com set as a fallback if the user closes the window first.
		if strings.Contains(uri, "login.vk.com") {
			loginCookies = cookie
			procPostQuitMessage.Call(0)
			return
		}
		vkCookies = cookie
		if !loginNavDone {
			loginNavDone = true
			chromium.Navigate("https://login.vk.com/")
		}
	}

	if !chromium.Embed(uintptr(hwnd)) {
		return "", "", fmt.Errorf("vklogin: WebView2 embed failed for user-data %q (Edge WebView2 Runtime installed?)", dataDir)
	}
	// Show the host window first, then make the WebView2 controller visible and size it to
	// the now-visible client area. Wails' controller starts hidden, so without Show() the
	// window stays blank white; and Resize must run against a shown window's client rect.
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	_ = chromium.Show()
	chromium.Resize()
	chromium.AddWebResourceRequestedFilter("*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
	chromium.Navigate(loginURL)
	log.Printf("vklogin: navigating to %s", loginURL)

	var msg win32msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	gotCookies := loginCookies
	if gotCookies == "" {
		gotCookies = vkCookies
	}
	if gotCookies == "" {
		return "", "", errors.New("vklogin: sign-in window closed before the VK session was captured")
	}
	if gotUA == "" {
		gotUA = userAgent
	}
	log.Printf("vklogin: captured VK session (login.vk.com set=%v, %d bytes)", loginCookies != "", len(gotCookies))
	return gotCookies, gotUA, nil
}

// reqHeader reads a single request header off a WebResourceRequested event's request.
func reqHeader(req *edge.ICoreWebView2WebResourceRequest, name string) string {
	h, err := req.GetHeaders()
	if err != nil || h == nil {
		return ""
	}
	defer h.Release()
	v, err := h.GetHeader(name)
	if err != nil {
		return ""
	}
	return v
}

type childResult struct {
	Cookies string `json:"cookies"`
	UA      string `json:"ua"`
}

// Login runs the VK sign-in in a SEPARATE process. A process may host only one WebView2
// environment, and the Wails app already owns one; so we re-exec ourselves with --vk-login
// to get a clean process whose only WebView2 is the sign-in window, and read the captured
// session back as JSON over the child's stdout.
func Login(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	cmd := exec.Command(exe, "--vk-login", loginURL, storageDir, userAgent)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("vklogin: sign-in helper: %w", err)
	}
	var res childResult
	if e := json.Unmarshal(out, &res); e != nil {
		return "", "", fmt.Errorf("vklogin: sign-in helper output: %w", e)
	}
	if res.Cookies == "" {
		return "", "", errors.New("vklogin: sign-in window closed before the VK session was captured")
	}
	return res.Cookies, res.UA, nil
}

// RunChild is the --vk-login entry point (called from main before any Wails setup). It
// captures the VK session in this single-WebView2 process and writes it to stdout as JSON
// for the parent Login to read; diagnostics go to stderr.
func RunChild(args []string) error {
	if len(args) < 3 {
		return errors.New("vklogin: --vk-login needs <url> <storageDir> <userAgent>")
	}
	cookies, ua, err := Capture(args[0], args[1], args[2])
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(childResult{Cookies: cookies, UA: ua})
}

// loginDataDir is the WebView2 user-data folder for the out-of-process VK sign-in. It must
// be writable and NOT the app's default WebView2 folder (which the main webview holds), so
// it lives under LocalAppData; the passed dir is the fallback.
func loginDataDir(storageDir string) string {
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		return filepath.Join(la, "WINGS V", "vk-login")
	}
	return storageDir
}

// ClearStore discards the captured VK session by removing the sign-in WebView2 folder (safe
// here: it belongs to the short-lived child process, not the running app) and the caller's
// dedicated store dir.
func ClearStore(storageDir string) error {
	_ = os.RemoveAll(loginDataDir(storageDir))
	if storageDir != "" {
		_ = os.RemoveAll(storageDir)
	}
	return nil
}
