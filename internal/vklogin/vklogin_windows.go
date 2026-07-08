//go:build windows

// Windows VK sign-in window via WebView2. The app's own webview cannot read HttpOnly
// cookies like remixsid, so this opens a dedicated WebView2 window and captures the VK
// web session by reading the "Cookie" (and "User-Agent") header off the browser's own
// outgoing requests to vk.com - those carry the full HttpOnly cookie set the relay needs.
//
// UNTESTED on a real Windows machine (written cross-platform): the WebView2 COM glue
// (reading request headers via reinterpreted vtables) and the Win32 window/message loop
// need verification on Windows. The COM vtable offsets follow the WebView2 IDL:
// ICoreWebView2WebResourceRequest.get_Headers is slot 9, ICoreWebView2HttpRequestHeaders
// .GetHeader is slot 3.
package vklogin

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
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

// COM vtable shapes we call by reinterpreting the WebView2 interface pointers.
type iunkVtbl struct{ queryInterface, addRef, release uintptr }

type reqVtbl struct {
	iunkVtbl
	getURI, putURI, getMethod, putMethod, getContent, putContent, getHeaders uintptr
}
type reqCOM struct{ vtbl *reqVtbl }

type hdrVtbl struct {
	iunkVtbl
	getHeader, getHeaders, contains, setHeader, removeHeader, getIterator uintptr
}
type hdrCOM struct{ vtbl *hdrVtbl }

// Capture opens the VK sign-in window and returns the captured "k=v; k=v" cookie header
// and the browser User-Agent once the VK session (remixsid) is present.
func Capture(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
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
	var gotCookies, gotUA string
	done := false

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
		return "", "", errors.New("vklogin: create window failed")
	}

	chromium = edge.NewChromium()
	if storageDir != "" {
		chromium.DataPath = storageDir
	}
	chromium.WebResourceRequestedCallback = func(req *edge.ICoreWebView2WebResourceRequest, _ *edge.ICoreWebView2WebResourceRequestedEventArgs) {
		if done {
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
		gotCookies = cookie
		done = true
		procPostQuitMessage.Call(0)
	}

	if !chromium.Embed(hwnd) {
		procDestroyWindow.Call(hwnd)
		return "", "", errors.New("vklogin: WebView2 embed failed (is the Microsoft Edge WebView2 runtime installed?)")
	}
	chromium.Resize()
	chromium.AddWebResourceRequestedFilter("*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
	chromium.Navigate(loginURL)
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	var msg win32msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	if gotCookies == "" {
		return "", "", errors.New("vklogin: sign-in window closed before the VK session was captured")
	}
	if gotUA == "" {
		gotUA = userAgent
	}
	return gotCookies, gotUA, nil
}

// reqHeader reads a request header off an ICoreWebView2WebResourceRequest by calling its
// get_Headers, then the header collection's GetHeader, via reinterpreted vtables.
func reqHeader(req *edge.ICoreWebView2WebResourceRequest, name string) string {
	obj := (*reqCOM)(unsafe.Pointer(req))
	var headersPtr uintptr
	if r, _, _ := edge.ComProc(obj.vtbl.getHeaders).Call(uintptr(unsafe.Pointer(req)), uintptr(unsafe.Pointer(&headersPtr))); r != 0 || headersPtr == 0 {
		return ""
	}
	h := (*hdrCOM)(unsafe.Pointer(headersPtr))
	defer edge.ComProc(h.vtbl.release).Call(headersPtr)

	namePtr, _ := windows.UTF16PtrFromString(name)
	var valPtr *uint16
	if r, _, _ := edge.ComProc(h.vtbl.getHeader).Call(headersPtr, uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&valPtr))); r != 0 || valPtr == nil {
		return ""
	}
	s := windows.UTF16PtrToString(valPtr)
	windows.CoTaskMemFree(unsafe.Pointer(valPtr))
	return s
}

// ClearStore removes the WebView2 user-data folder, discarding the captured VK session.
func ClearStore(storageDir string) error {
	if storageDir == "" {
		return nil
	}
	return os.RemoveAll(storageDir)
}
