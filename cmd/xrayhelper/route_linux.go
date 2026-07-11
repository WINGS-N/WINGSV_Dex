//go:build linux

package main

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

func initIpRoute(tunName string, tunPriority int, enableIPv6 bool) error {
	var link netlink.Link
	err := retryRouteInitStep("find tun device "+tunName, func() error {
		var e error
		link, e = netlink.LinkByName(tunName)
		return e
	})
	if err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("set %s up failed: %w", tunName, err)
	}
	if err := addAddress(link, defaultTunIPv4Address); err != nil {
		return err
	}
	if err := addRoute(link.Attrs().Index, defaultIPv4Route, defaultTunIPv4Gateway, netlink.FAMILY_V4, tunPriority); err != nil {
		return err
	}
	if enableIPv6 {
		if err := addAddress(link, defaultTunIPv6Address); err != nil {
			return err
		}
		if err := addRoute(link.Attrs().Index, defaultIPv6Route, defaultTunIPv6Gateway, netlink.FAMILY_V6, tunPriority); err != nil {
			return err
		}
	}
	return nil
}

func addAddress(link netlink.Link, address string) error {
	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", address, err)
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("add address %q failed: %w", address, err)
	}
	return nil
}

func addRoute(index int, cidr string, gateway string, family int, priority int) error {
	_, dst, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	gw := net.ParseIP(gateway)
	if gw == nil {
		return fmt.Errorf("invalid gateway %q", gateway)
	}
	route := netlink.Route{Dst: dst, Gw: gw, LinkIndex: index, Family: family, Priority: priority}
	return netlink.RouteAdd(&route)
}
