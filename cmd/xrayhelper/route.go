package main

import (
	"fmt"
	"time"
)

// The TUN uses the CGNAT-ish 198.18.0.0/15 block and a link-local gateway; this matches the
// libXray desktop harness so the fork's proxy/tun inbound and this routing agree.
const (
	routeInitRetryAttempts = 3
	routeInitRetryInterval = 2 * time.Second
	defaultTunIPv4Address  = "198.18.0.1/15"
	defaultTunIPv4Gateway  = "198.18.0.2"
	defaultIPv4Route       = "0.0.0.0/0"
	defaultTunIPv6Address  = "fc00::1/64"
	defaultTunIPv6Gateway  = "fc00::2"
	defaultIPv6Route       = "::/0"
)

// The TUN device appears a moment after xray starts, so finding it is retried briefly.
func retryRouteInitStep(step string, fn func() error) error {
	var err error
	for i := 1; i <= routeInitRetryAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < routeInitRetryAttempts {
			time.Sleep(routeInitRetryInterval)
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", step, routeInitRetryAttempts, err)
}
