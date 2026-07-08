//go:build !windows

package vklogin

import "errors"

// Login runs the VK sign-in and returns the captured session. Off Windows the capture runs
// in-process (WebKitGTK on Linux has no single-environment-per-process restriction).
func Login(loginURL, storageDir, userAgent string) (cookies, ua string, err error) {
	return Capture(loginURL, storageDir, userAgent)
}

// RunChild is the --vk-login re-exec entry point; only Windows needs an out-of-process
// sign-in (its WebView2 allows one environment per process, and the app owns it already).
func RunChild(args []string) error {
	return errors.New("vklogin: --vk-login child mode is Windows-only")
}
