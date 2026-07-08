//go:build !linux && !windows

package nethelper

import "errors"

// Run is implemented on Linux (kernel WireGuard) and Windows (wintun + wireguard-go).
func Run() error {
	return errors.New("net-helper is not supported on this platform")
}
