//go:build windows

// Package wgwin is the Windows WireGuard data plane: a userspace wireguard-go device on
// a wintun adapter, configured over the UAPI, with addresses/routes/DNS set via
// winipcfg. It is the Windows analogue of the Linux kernel-WireGuard path in package wg.
// Everything here needs administrator rights, so it runs inside the elevated net-helper.
package wgwin

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// Config mirrors the data-plane wire config (dataplane.WGConfig) for the fields the
// Windows path needs. Keys are standard base64 (WireGuard's own encoding).
type Config struct {
	Interface     string
	PrivateKey    string
	Addresses     []string // CIDRs, e.g. "10.8.0.2/32"
	MTU           int
	PeerPublicKey string
	PresharedKey  string
	PeerEndpoint  string // "127.0.0.1:9000"
	AllowedIPs    []string
	DNS           []string
	Amnezia       bool
}

// Tunnel is a live wintun + wireguard-go interface. Close it with Down.
type Tunnel struct {
	dev  *device.Device
	tun  tun.Device
	luid winipcfg.LUID
}

// Up creates the wintun adapter, starts the userspace WireGuard device, configures the
// single peer and sets the interface addresses, routes and DNS.
func Up(cfg Config) (*Tunnel, error) {
	if cfg.Amnezia {
		return nil, fmt.Errorf("wgwin: AmneziaWG is not supported on Windows yet (needs a patched wireguard-go)")
	}
	name := cfg.Interface
	if name == "" {
		name = "wingsv0"
	}
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1280
	}

	uapi, err := uapiConfig(cfg)
	if err != nil {
		return nil, err
	}

	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, fmt.Errorf("wgwin: create wintun adapter: %w", err)
	}
	nativeTun := tunDev.(*tun.NativeTun)
	luid := winipcfg.LUID(nativeTun.LUID())

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "wingsv-wg: "))
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wgwin: configure device: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("wgwin: bring device up: %w", err)
	}

	t := &Tunnel{dev: dev, tun: tunDev, luid: luid}
	if err := t.applyInterface(cfg); err != nil {
		t.Down()
		return nil, err
	}
	return t, nil
}

// Down tears the interface down; closing the device removes the wintun adapter and its
// routes.
func (t *Tunnel) Down() {
	if t == nil {
		return
	}
	if t.dev != nil {
		t.dev.Close()
		t.dev = nil
	}
}

func (t *Tunnel) applyInterface(cfg Config) error {
	addrs, err := parsePrefixes(cfg.Addresses)
	if err != nil {
		return fmt.Errorf("wgwin: addresses: %w", err)
	}
	if err := t.luid.SetIPAddresses(addrs); err != nil {
		return fmt.Errorf("wgwin: set addresses: %w", err)
	}

	allowed, err := parsePrefixes(cfg.AllowedIPs)
	if err != nil {
		return fmt.Errorf("wgwin: allowed ips: %w", err)
	}
	routes := make([]*winipcfg.RouteData, 0, len(allowed))
	for _, p := range allowed {
		routes = append(routes, &winipcfg.RouteData{
			Destination: p.Masked(),
			NextHop:     zeroAddr(p),
			Metric:      0,
		})
	}
	if err := t.luid.SetRoutes(routes); err != nil {
		return fmt.Errorf("wgwin: set routes: %w", err)
	}

	if len(cfg.DNS) > 0 {
		var v4, v6 []netip.Addr
		for _, s := range cfg.DNS {
			a, err := netip.ParseAddr(strings.TrimSpace(s))
			if err != nil {
				continue
			}
			if a.Is4() {
				v4 = append(v4, a)
			} else {
				v6 = append(v6, a)
			}
		}
		if len(v4) > 0 {
			_ = t.luid.SetDNS(windows.AF_INET, v4, nil)
		}
		if len(v6) > 0 {
			_ = t.luid.SetDNS(windows.AF_INET6, v6, nil)
		}
	}
	return nil
}

// uapiConfig renders the wireguard-go UAPI configuration string. UAPI uses hex-encoded
// keys, unlike the base64 form on the wire, so the keys are re-encoded here.
func uapiConfig(cfg Config) (string, error) {
	priv, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("wgwin: private key: %w", err)
	}
	pub, err := wgtypes.ParseKey(cfg.PeerPublicKey)
	if err != nil {
		return "", fmt.Errorf("wgwin: peer public key: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(priv[:]))
	fmt.Fprintf(&b, "replace_peers=true\n")
	fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(pub[:]))
	if strings.TrimSpace(cfg.PresharedKey) != "" {
		psk, err := wgtypes.ParseKey(cfg.PresharedKey)
		if err != nil {
			return "", fmt.Errorf("wgwin: preshared key: %w", err)
		}
		fmt.Fprintf(&b, "preshared_key=%s\n", hex.EncodeToString(psk[:]))
	}
	fmt.Fprintf(&b, "endpoint=%s\n", cfg.PeerEndpoint)
	fmt.Fprintf(&b, "persistent_keepalive_interval=25\n")
	fmt.Fprintf(&b, "replace_allowed_ips=true\n")
	allowed := cfg.AllowedIPs
	if len(allowed) == 0 {
		allowed = []string{"0.0.0.0/0", "::/0"}
	}
	for _, a := range allowed {
		if _, err := netip.ParsePrefix(strings.TrimSpace(a)); err != nil {
			return "", fmt.Errorf("wgwin: allowed ip %q: %w", a, err)
		}
		fmt.Fprintf(&b, "allowed_ip=%s\n", strings.TrimSpace(a))
	}
	return b.String(), nil
}

func parsePrefixes(cidrs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(strings.TrimSpace(c))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func zeroAddr(p netip.Prefix) netip.Addr {
	if p.Addr().Is4() {
		return netip.IPv4Unspecified()
	}
	return netip.IPv6Unspecified()
}
