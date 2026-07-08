//go:build windows

// Package wgwin is the Windows WireGuard data plane: a userspace wireguard-go device on
// a wintun adapter, configured over the UAPI, with addresses/routes/DNS set via
// winipcfg. It is the Windows analogue of the Linux kernel-WireGuard path in package wg.
// Everything here needs administrator rights, so it runs inside the elevated net-helper.
package wgwin

import (
	"encoding/hex"
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"strings"
	"syscall"

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
	dev          *device.Device
	tun          tun.Device
	luid         winipcfg.LUID
	gwIP         netip.Addr            // physical default-route gateway (resolved before wintun)
	gwLUID       winipcfg.LUID         // physical interface LUID
	routes       []*winipcfg.RouteData // full-tunnel catch-all routes, installed by Activate
	bypassRoutes []netip.Prefix        // /32 underlay routes added on gwLUID, removed on Down
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

	// Resolve the default-route gateway BEFORE wintun exists (afterwards it would resolve to
	// the tunnel). Used for the /32 underlay-bypass routes vkturn asks for after connect.
	gwIP, gwLUID := physicalGateway()

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

	t := &Tunnel{dev: dev, tun: tunDev, luid: luid, gwIP: gwIP, gwLUID: gwLUID}
	if err := t.applyInterface(cfg); err != nil {
		t.Down()
		return nil, err
	}
	return t, nil
}

// physicalGateway returns the next hop and interface LUID of the current IPv4 default
// route. Call it before wintun exists, otherwise it may return the tunnel.
func physicalGateway() (netip.Addr, winipcfg.LUID) {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return netip.Addr{}, 0
	}
	var best *winipcfg.MibIPforwardRow2
	for i := range rows {
		r := &rows[i]
		if r.DestinationPrefix.PrefixLength != 0 {
			continue // only 0.0.0.0/0
		}
		if best == nil || r.Metric < best.Metric {
			best = r
		}
	}
	if best == nil {
		return netip.Addr{}, 0
	}
	return best.NextHop.Addr(), best.InterfaceLUID
}

// AddBypassRoute routes a single underlay destination (a VK TURN / peer server IP vkturn
// connects to) out the physical gateway with a /32, overriding the full-tunnel /1 routes.
// Without it vkturn's own traffic matches the tunnel /1 routes and gets the tunnel's source
// address (a martian on the physical link that the upstream router drops), so the underlay
// "sends but never gets a reply". Idempotent per IP; best effort.
func (t *Tunnel) AddBypassRoute(ipStr string) {
	if t == nil || !t.gwIP.IsValid() || t.gwLUID == 0 {
		return
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !ip.Is4() {
		return
	}
	dst := netip.PrefixFrom(ip, 32)
	t.bypassRoutes = append(t.bypassRoutes, dst)
	if err := t.gwLUID.AddRoute(dst, t.gwIP, 1); err != nil {
		// "already exists" is fine (a stale route from a prior run, or a duplicate report).
		log.Printf("wgwin: bypass route %s via %s: %v", ip, t.gwIP, err)
		return
	}
	log.Printf("wgwin: bypass %s via %s", ip, t.gwIP)
}

// Activate installs the deferred full-tunnel catch-all routes. Call it AFTER the underlay
// /32 bypass routes are in place so the tunnel captures everything except the already-
// bypassed underlay, and established underlay sockets are never re-routed mid-flight.
func (t *Tunnel) Activate() {
	if t == nil || len(t.routes) == 0 {
		return
	}
	if err := t.luid.SetRoutes(t.routes); err != nil {
		log.Printf("wgwin: activate: set routes: %v", err)
	}
	t.addRoutesViaNetsh(t.routes)
	log.Printf("wgwin: activated full tunnel (%d routes)", len(t.routes))
}

// Down tears the interface down: it removes the /32 underlay bypass routes it added on the
// physical interface (closing the device only drops the tunnel's own routes) and then
// closes the wireguard-go device, which removes the wintun adapter.
func (t *Tunnel) Down() {
	if t == nil {
		return
	}
	for _, dst := range t.bypassRoutes {
		_ = t.gwLUID.DeleteRoute(dst, t.gwIP)
	}
	t.bypassRoutes = nil
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

	// Pin the interface metric low and off automatic, like the WireGuard Windows client,
	// so the tunnel routes win deterministically.
	for _, fam := range []winipcfg.AddressFamily{windows.AF_INET, windows.AF_INET6} {
		if iface, err := t.luid.IPInterface(fam); err == nil {
			iface.UseAutomaticMetric = false
			iface.Metric = 0
			if err := iface.Set(); err != nil {
				log.Printf("wgwin: set ipinterface metric family=%d: %v", fam, err)
			}
		}
	}

	allowed, err := parsePrefixes(cfg.AllowedIPs)
	if err != nil {
		return fmt.Errorf("wgwin: allowed ips: %w", err)
	}
	routes := make([]*winipcfg.RouteData, 0, len(allowed)+2)
	for _, p := range allowed {
		m := p.Masked()
		// A single 0.0.0.0/0 route on wintun competes with the physical default route by
		// metric only, which is unreliable on Windows. Split it into two /1 routes (more
		// specific than /0) so the tunnel wins over the physical default without deleting
		// it - the same trick wg-quick / the WireGuard Windows client use.
		if m.Bits() == 0 {
			if m.Addr().Is4() {
				routes = append(routes, splitRoute("0.0.0.0/1"), splitRoute("128.0.0.0/1"))
			} else {
				routes = append(routes, splitRoute("::/1"), splitRoute("8000::/1"))
			}
			continue
		}
		routes = append(routes, &winipcfg.RouteData{Destination: m, NextHop: zeroAddr(m), Metric: 0})
	}
	// Defer installing the catch-all routes until Activate. The host adds the underlay /32
	// bypass routes first (two-phase, like wg-quick's endpoint route before the default), so
	// vkturn's already-established underlay sockets are not re-routed mid-flight - that would
	// change their NAT mapping and drop the TURN allocations, tearing the streams down.
	t.routes = routes

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

func splitRoute(cidr string) *winipcfg.RouteData {
	p := netip.MustParsePrefix(cidr)
	return &winipcfg.RouteData{Destination: p, NextHop: zeroAddr(p), Metric: 0}
}

// addRoutesViaNetsh mirrors each tunnel route with a netsh command keyed by the interface
// index, as a fallback for winipcfg SetRoutes silently no-oping in the elevated hidden
// net-helper. Best effort: failures are logged, not fatal.
func (t *Tunnel) addRoutesViaNetsh(routes []*winipcfg.RouteData) {
	row, err := t.luid.Interface()
	if err != nil {
		log.Printf("wgwin: netsh routes: resolve interface index: %v", err)
		return
	}
	idx := row.InterfaceIndex
	for _, r := range routes {
		fam := "ipv4"
		if r.Destination.Addr().Is6() {
			fam = "ipv6"
		}
		out, err := runHidden("netsh", "interface", fam, "add", "route",
			"prefix="+r.Destination.String(), fmt.Sprintf("interface=%d", idx), "store=active")
		if err != nil {
			log.Printf("wgwin: netsh route %s: %v (%s)", r.Destination, err, strings.TrimSpace(string(out)))
		}
	}
}

// runHidden runs a console command (netsh) without flashing a console window.
func runHidden(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.CombinedOutput()
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
