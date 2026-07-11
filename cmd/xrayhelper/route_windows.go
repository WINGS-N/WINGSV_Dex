//go:build windows

package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"syscall"
)

func initIpRoute(tunName string, tunPriority int, enableIPv6 bool) error {
	err := retryRouteInitStep("find tun device "+tunName, func() error {
		_, e := net.InterfaceByName(tunName)
		return e
	})
	if err != nil {
		return err
	}
	if err := addAddress(tunName, "ipv4", defaultTunIPv4Address); err != nil {
		return err
	}
	if err := addRoute(tunName, "ipv4", defaultIPv4Route, defaultTunIPv4Gateway, tunPriority); err != nil {
		return err
	}
	if enableIPv6 {
		if err := addAddress(tunName, "ipv6", defaultTunIPv6Address); err != nil {
			return err
		}
		if err := addRoute(tunName, "ipv6", defaultIPv6Route, defaultTunIPv6Gateway, tunPriority); err != nil {
			return err
		}
	}
	return nil
}

func netsh(args ...string) error {
	cmd := exec.Command("netsh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh %v failed: %s: %w", args, string(out), err)
	}
	return nil
}

func addAddress(tunName string, ipVersion string, address string) error {
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("invalid %s address %q: %w", ipVersion, address, err)
	}
	switch ipVersion {
	case "ipv4":
		if ip.To4() == nil {
			return fmt.Errorf("ipv4 address must be an IPv4 CIDR: %q", address)
		}
		mask := net.IP(ipNet.Mask).String()
		return netsh("interface", "ipv4", "add", "address", "name="+tunName,
			"address="+ip.String(), "mask="+mask, "store=active")
	case "ipv6":
		if ip.To4() != nil {
			return fmt.Errorf("ipv6 address must be an IPv6 CIDR: %q", address)
		}
		return netsh("interface", "ipv6", "add", "address", "interface="+tunName,
			"address="+address, "store=active")
	default:
		return fmt.Errorf("unsupported ip version %q", ipVersion)
	}
}

func addRoute(tunName string, ipVersion string, prefix string, gateway string, metric int) error {
	routeIP, _, err := net.ParseCIDR(prefix)
	if err != nil {
		return fmt.Errorf("invalid %s route prefix %q: %w", ipVersion, prefix, err)
	}
	args := []string{"interface", ipVersion, "add", "route", "prefix=" + prefix, "interface=" + tunName}
	switch ipVersion {
	case "ipv4":
		if routeIP.To4() == nil {
			return fmt.Errorf("ipv4 route prefix must be an IPv4 CIDR: %q", prefix)
		}
		if gateway != "" {
			if gw := net.ParseIP(gateway); gw == nil || gw.To4() == nil {
				return fmt.Errorf("ipv4 route gateway must be an IPv4 address: %q", gateway)
			}
			args = append(args, "nexthop="+gateway)
		}
	case "ipv6":
		if routeIP.To4() != nil {
			return fmt.Errorf("ipv6 route prefix must be an IPv6 CIDR: %q", prefix)
		}
		if gateway != "" {
			if gw := net.ParseIP(gateway); gw == nil || gw.To4() != nil {
				return fmt.Errorf("ipv6 route gateway must be an IPv6 address: %q", gateway)
			}
			args = append(args, "nexthop="+gateway)
		}
	default:
		return fmt.Errorf("unsupported ip version %q", ipVersion)
	}
	args = append(args, "metric="+strconv.Itoa(metric), "store=active")
	return netsh(args...)
}
