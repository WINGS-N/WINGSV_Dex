//go:build linux

// Package wg brings a kernel WireGuard interface up and down for the VK TURN data
// plane and installs the policy routing that keeps the vkturn underlay traffic out
// of the tunnel. Every operation here needs CAP_NET_ADMIN, so it runs inside the
// privileged net-helper, not the main app process.
package wg

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Config is a resolved WireGuard data-plane setup. Keys are standard base64
// (WireGuard's own encoding); PeerEndpoint is the vkturn local listen address.
type Config struct {
	Interface     string   `json:"interface"`
	PrivateKey    string   `json:"privateKey"`
	Addresses     []string `json:"addresses"` // CIDRs, e.g. "10.8.0.2/32"
	MTU           int      `json:"mtu"`
	PeerPublicKey string   `json:"peerPublicKey"`
	PresharedKey  string   `json:"presharedKey"`
	PeerEndpoint  string   `json:"peerEndpoint"` // "127.0.0.1:9000"
	AllowedIPs    []string `json:"allowedIps"`   // "0.0.0.0/0", "::/0"
	FwMark        int      `json:"fwmark"`       // marks WG transport + protected vkturn sockets to bypass the tunnel
	Table         int      `json:"table"`        // policy routing table for the tunnel default route

	// Whitelist inverts the routing for app-routing whitelist mode: instead of
	// "unmarked -> tunnel", only traffic marked with AppsMark (the whitelisted apps'
	// cgroup) is sent to the tunnel table, so everything else stays on the physical
	// link. Off/bypass mode leaves this false (unmarked -> tunnel, marked bypasses).
	Whitelist bool `json:"whitelist"`

	// Amnezia selects the AmneziaWG data plane (amneziawg kernel module + `awg` tool)
	// instead of stock kernel WireGuard; the junk params below then obfuscate the
	// handshake. Empty AWG fields are omitted from the awg config.
	Amnezia bool   `json:"amnezia"`
	Jc      string `json:"jc"`
	Jmin    string `json:"jmin"`
	Jmax    string `json:"jmax"`
	S1      string `json:"s1"`
	S2      string `json:"s2"`
	S3      string `json:"s3"`
	S4      string `json:"s4"`
	H1      string `json:"h1"`
	H2      string `json:"h2"`
	H3      string `json:"h3"`
	H4      string `json:"h4"`
}

// Up creates the interface, configures WireGuard, assigns the address, and installs
// the tunnel routes and fwmark bypass rules (wg-quick style). It is idempotent:
// a stale interface from a previous run is torn down first.
func Up(cfg Config) error {
	if cfg.Amnezia {
		return UpAWG(cfg)
	}
	_ = Down(cfg)

	link := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: cfg.Interface}}
	if err := netlink.LinkAdd(link); err != nil {
		return fmt.Errorf("wg: create %s: %w", cfg.Interface, err)
	}
	if err := configureDevice(cfg); err != nil {
		_ = netlink.LinkDel(link)
		return err
	}
	return finishUp(cfg, link)
}

// UpAWG brings up an AmneziaWG interface. The amneziawg genetlink family is not
// spoken by wgctrl, so the link is created via `ip link add type amneziawg` and keyed
// via the `awg` tool (both from amneziawg-tools + the amneziawg kernel module); the
// rest (addr, mtu, routes, fwmark policy) is identical to the WireGuard path.
func UpAWG(cfg Config) error {
	_ = Down(cfg)
	if out, err := exec.Command("ip", "link", "add", "name", cfg.Interface, "type", "amneziawg").CombinedOutput(); err != nil {
		return fmt.Errorf("wg: create amneziawg %s (amneziawg-tools/kernel module missing?): %w: %s",
			cfg.Interface, err, strings.TrimSpace(string(out)))
	}
	if err := awgSetConf(cfg); err != nil {
		_ = exec.Command("ip", "link", "del", cfg.Interface).Run()
		return err
	}
	link, err := netlink.LinkByName(cfg.Interface)
	if err != nil {
		_ = exec.Command("ip", "link", "del", cfg.Interface).Run()
		return fmt.Errorf("wg: amneziawg link %s: %w", cfg.Interface, err)
	}
	return finishUp(cfg, link)
}

// finishUp applies the MTU, addresses, link-up and policy routing shared by the
// WireGuard and AmneziaWG paths.
func finishUp(cfg Config, link netlink.Link) error {
	if cfg.MTU > 0 {
		if err := netlink.LinkSetMTU(link, cfg.MTU); err != nil {
			_ = Down(cfg)
			return fmt.Errorf("wg: set mtu: %w", err)
		}
	}
	for _, addr := range cfg.Addresses {
		a, err := netlink.ParseAddr(strings.TrimSpace(addr))
		if err != nil {
			_ = Down(cfg)
			return fmt.Errorf("wg: parse address %q: %w", addr, err)
		}
		if err := netlink.AddrAdd(link, a); err != nil {
			_ = Down(cfg)
			return fmt.Errorf("wg: add address %q: %w", addr, err)
		}
	}
	if err := netlink.LinkSetUp(link); err != nil {
		_ = Down(cfg)
		return fmt.Errorf("wg: link up: %w", err)
	}
	if err := installRouting(cfg, link); err != nil {
		_ = Down(cfg)
		return err
	}
	return nil
}

// awgSetConf writes an `awg setconf` file (wg-style config plus the Amnezia junk
// params) and applies it. The private key rides a 0600 temp file removed right after.
func awgSetConf(cfg Config) error {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", cfg.PrivateKey)
	fmt.Fprintf(&b, "FwMark = %d\n", cfg.FwMark)
	for k, v := range map[string]string{
		"Jc": cfg.Jc, "Jmin": cfg.Jmin, "Jmax": cfg.Jmax,
		"S1": cfg.S1, "S2": cfg.S2, "S3": cfg.S3, "S4": cfg.S4,
		"H1": cfg.H1, "H2": cfg.H2, "H3": cfg.H3, "H4": cfg.H4,
	} {
		if strings.TrimSpace(v) != "" {
			fmt.Fprintf(&b, "%s = %s\n", k, strings.TrimSpace(v))
		}
	}
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", cfg.PeerPublicKey)
	if strings.TrimSpace(cfg.PresharedKey) != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", cfg.PresharedKey)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", cfg.PeerEndpoint)
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(cfg.AllowedIPs, ", "))
	b.WriteString("PersistentKeepalive = 25\n")

	f, err := os.CreateTemp("", "wingsv-awg-*.conf")
	if err != nil {
		return fmt.Errorf("wg: awg conf temp: %w", err)
	}
	defer os.Remove(f.Name())
	_ = f.Chmod(0o600)
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return fmt.Errorf("wg: write awg conf: %w", err)
	}
	_ = f.Close()
	if out, err := exec.Command("awg", "setconf", cfg.Interface, f.Name()).CombinedOutput(); err != nil {
		return fmt.Errorf("wg: awg setconf: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Down removes the fwmark rules and deletes the interface (which drops its routes).
func Down(cfg Config) error {
	removeRules(cfg)
	_ = SetTunnelMasquerade(false, cfg.Interface, AppsMark)
	if link, err := netlink.LinkByName(cfg.Interface); err == nil {
		return netlink.LinkDel(link)
	}
	return nil
}

func configureDevice(cfg Config) error {
	priv, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("wg: private key: %w", err)
	}
	pub, err := wgtypes.ParseKey(cfg.PeerPublicKey)
	if err != nil {
		return fmt.Errorf("wg: peer public key: %w", err)
	}
	endpoint, err := net.ResolveUDPAddr("udp", cfg.PeerEndpoint)
	if err != nil {
		return fmt.Errorf("wg: peer endpoint %q: %w", cfg.PeerEndpoint, err)
	}
	allowed, err := parseCIDRs(cfg.AllowedIPs)
	if err != nil {
		return err
	}
	peer := wgtypes.PeerConfig{
		PublicKey:         pub,
		Endpoint:          endpoint,
		ReplaceAllowedIPs: true,
		AllowedIPs:        allowed,
	}
	if strings.TrimSpace(cfg.PresharedKey) != "" {
		psk, err := wgtypes.ParseKey(cfg.PresharedKey)
		if err != nil {
			return fmt.Errorf("wg: preshared key: %w", err)
		}
		peer.PresharedKey = &psk
	}
	mark := cfg.FwMark
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wg: wgctrl: %w", err)
	}
	defer client.Close()
	return client.ConfigureDevice(cfg.Interface, wgtypes.Config{
		PrivateKey:   &priv,
		FirewallMark: &mark,
		ReplacePeers: true,
		Peers:        []wgtypes.PeerConfig{peer},
	})
}

func parseCIDRs(cidrs []string) ([]net.IPNet, error) {
	out := make([]net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("wg: parse allowed ip %q: %w", c, err)
		}
		out = append(out, *ipnet)
	}
	return out, nil
}

// installRouting sets up wg-quick-style full-tunnel policy routing per address
// family present in AllowedIPs: a default route in the tunnel table, a "not fwmark"
// rule sending unmarked traffic there, and a main-table suppress rule so local /
// LAN routes still win. Marked traffic (WG transport + protected vkturn sockets)
// skips the rule and uses the main table, staying off the tunnel.
func installRouting(cfg Config, link netlink.Link) error {
	v4, v6 := families(cfg.AllowedIPs)
	if v4 {
		// Without this, reverse-path filtering drops the fwmark-bypassed packets
		// (WG transport + protected vkturn sockets) because their source address
		// looks invalid for the chosen route. wg-quick sets it for the same reason.
		if err := os.WriteFile("/proc/sys/net/ipv4/conf/all/src_valid_mark", []byte("1"), 0o644); err != nil {
			return fmt.Errorf("wg: enable src_valid_mark: %w", err)
		}
		// Loose reverse-path filtering on the tunnel interface. In app-routing
		// whitelist mode the default route is the physical link, so a decrypted
		// reply arriving on the tunnel (source = an internet host) fails strict
		// rp_filter (that host routes via the physical link, not the tunnel) and the
		// kernel drops it - the tunnel sends but never receives. Loose (2) accepts a
		// source reachable via any interface. Effective rp_filter is max(all, iface),
		// and max(anything<=2, 2) == 2, so setting the interface alone suffices.
		if err := os.WriteFile("/proc/sys/net/ipv4/conf/"+link.Attrs().Name+"/rp_filter", []byte("2"), 0o644); err != nil {
			return fmt.Errorf("wg: set loose rp_filter: %w", err)
		}
		if err := addFamilyRouting(cfg, link, netlink.FAMILY_V4, "0.0.0.0/0"); err != nil {
			return err
		}
	}
	if v6 {
		if err := addFamilyRouting(cfg, link, netlink.FAMILY_V6, "::/0"); err != nil {
			return err
		}
	}
	if cfg.Whitelist {
		if err := SetTunnelMasquerade(true, link.Attrs().Name, AppsMark); err != nil {
			return err
		}
	}
	return nil
}

func addFamilyRouting(cfg Config, link netlink.Link, family int, defaultCIDR string) error {
	_, dst, err := net.ParseCIDR(defaultCIDR)
	if err != nil {
		return err
	}
	if err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       dst,
		Table:     cfg.Table,
	}); err != nil {
		return fmt.Errorf("wg: default route (family %d): %w", family, err)
	}

	if err := netlink.RuleAdd(tunnelRule(cfg, family)); err != nil {
		return fmt.Errorf("wg: fwmark rule (family %d): %w", family, err)
	}

	suppress := netlink.NewRule()
	suppress.Family = family
	suppress.Table = mainTable
	suppress.SuppressPrefixlen = 0
	suppress.Priority = rulePriority - 1
	if err := netlink.RuleAdd(suppress); err != nil {
		return fmt.Errorf("wg: suppress rule (family %d): %w", family, err)
	}
	return nil
}

// tunnelRule selects which traffic reaches the tunnel table. Bypass/off: everything
// NOT bearing the bypass fwmark (marked = vkturn + bypass apps stay on the physical
// link). Whitelist: ONLY traffic bearing AppsMark (the whitelisted apps), so the
// unmarked default stays direct.
func tunnelRule(cfg Config, family int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.Table = cfg.Table
	r.Priority = rulePriority
	if cfg.Whitelist {
		r.Mark = uint32(AppsMark)
		r.Invert = false
	} else {
		r.Mark = uint32(cfg.FwMark)
		r.Invert = true
	}
	return r
}

// SwapTunnelRule replaces the tunnel policy-routing rule to match a new app-routing
// mode (whitelist toggled), without touching the live interface, so the mode can be
// changed on the fly. The interface, default route and suppress rule are unchanged.
func SwapTunnelRule(cfg Config, oldWL, newWL bool) error {
	if oldWL == newWL {
		return nil
	}
	v4, v6 := families(cfg.AllowedIPs)
	var fams []int
	if v4 {
		fams = append(fams, netlink.FAMILY_V4)
	}
	if v6 {
		fams = append(fams, netlink.FAMILY_V6)
	}
	for _, family := range fams {
		old := cfg
		old.Whitelist = oldWL
		_ = netlink.RuleDel(tunnelRule(old, family))
		neu := cfg
		neu.Whitelist = newWL
		if err := netlink.RuleAdd(tunnelRule(neu, family)); err != nil {
			return fmt.Errorf("wg: swap tunnel rule (family %d): %w", family, err)
		}
	}
	// Masquerade is whitelist-only (see SetTunnelMasquerade); toggle it with the mode.
	if err := SetTunnelMasquerade(newWL, cfg.Interface, AppsMark); err != nil {
		return err
	}
	return nil
}

func removeRules(cfg Config) {
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		_ = netlink.RuleDel(tunnelRule(cfg, family))

		suppress := netlink.NewRule()
		suppress.Family = family
		suppress.Table = mainTable
		suppress.SuppressPrefixlen = 0
		suppress.Priority = rulePriority - 1
		_ = netlink.RuleDel(suppress)
	}
}

func families(cidrs []string) (v4, v6 bool) {
	for _, c := range cidrs {
		ip, _, err := net.ParseCIDR(strings.TrimSpace(c))
		if err != nil {
			continue
		}
		if ip.To4() != nil {
			v4 = true
		} else {
			v6 = true
		}
	}
	return v4, v6
}

const (
	mainTable    = 254
	rulePriority = 18800
	// AppsMark tags the whitelisted apps' cgroup so tunnelRule can send only that
	// traffic into the tunnel (whitelist mode). Distinct from the bypass FwMark.
	AppsMark = 0x8889
)
