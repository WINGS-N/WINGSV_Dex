//go:build !linux && !windows

package vklogin

import "errors"

// Capture is only implemented on Linux (WebKitGTK); other platforms would use their
// native cookie store (WKHTTPCookieStore on macOS, ICoreWebView2CookieManager on
// Windows). See the wails-v3-webview-cookie-limitation note.
func Capture(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
	return "", "", errors.New("vklogin: VK sign-in window is only implemented on Linux")
}

// ClearStore is a no-op off Linux.
func ClearStore(storageDir string) error { return nil }
