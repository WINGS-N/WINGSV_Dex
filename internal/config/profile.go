package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

// Profile is a self-contained VK TURN profile: the flat settings snapshot plus a
// resolved WireGuard transport. On Android the transport is by-reference into a
// shared library; on desktop we resolve and embed it, since a shared wingsv:// link
// carries the referenced transport inside the same Config.
type Profile struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	VKTurnEndpoint    string    `json:"vkTurnEndpoint"`
	TransportKind     string    `json:"transportKind"` // "wg" | "awg"
	Links             []string  `json:"links"`         // VK call links the relay mints TURN creds from
	LinkSecondary     string    `json:"linkSecondary,omitempty"`
	Favorite          bool      `json:"favorite"`
	Managed           bool      `json:"managed"` // WG provisioned by the node on connect
	ProvisionClientID string    `json:"provisionClientId,omitempty"`
	ProvisionToken    string    `json:"provisionToken,omitempty"`
	Settings          Settings  `json:"settings"`
	WG                WireGuard `json:"wg"`
}

// DedupKey identifies the same server across re-imports via its server identity
// (transport + VK TURN endpoint + peer key).
func (p Profile) DedupKey() string {
	return strings.Join([]string{p.TransportKind, p.VKTurnEndpoint, p.WG.PublicKey, p.ProvisionClientID}, "|")
}

// ProfilesFromConfig extracts every VK TURN profile carried by a decoded Config.
// A single-profile share link yields one; a multi-profile Config yields several.
func ProfilesFromConfig(cfg *wingsvpb.Config) []Profile {
	turn := cfg.GetTurn()
	if turn == nil {
		return nil
	}
	topEndpoint := endpointString(turn.GetEndpoint())
	if topEndpoint == "" {
		topEndpoint = hostPort(turn.GetHost(), turn.GetPort())
	}

	tps := turn.GetProfiles()
	if len(tps) == 0 {
		s := DefaultSettings()
		overlayTurn(&s, turn)
		return []Profile{buildProfile(profileParts{
			endpoint:      topEndpoint,
			title:         firstNonEmpty(strings.TrimSpace(turn.GetTitle()), topEndpoint, "VK TURN"),
			transportKind: tunnelKind(turn.GetTunnelMode()),
			links:         turnLinks(turn),
			linkSecondary: strings.TrimSpace(turn.GetLinkSecondary()),
			settings:      s,
			wg:            wgFromProto(cfg.GetWg()),
		})}
	}

	out := make([]Profile, 0, len(tps))
	for _, tp := range tps {
		s := DefaultSettings()
		overlayTurn(&s, turn)
		overlayTurn(&s, tp.GetConfig())
		endpoint := firstNonEmpty(strings.TrimSpace(tp.GetVkTurnEndpoint()), topEndpoint)
		kind := firstNonEmpty(strings.TrimSpace(tp.GetTransportKind()), tunnelKind(turn.GetTunnelMode()))
		if mode := strings.TrimSpace(tp.GetVkAuthMode()); mode != "" {
			s.VKAuthMode = mode
		}
		if mode := strings.TrimSpace(tp.GetDnsMode()); mode != "" {
			s.DNSMode = mode
		}
		links := turnLinks(tp.GetConfig())
		if len(links) == 0 {
			links = turnLinks(turn)
		}
		out = append(out, buildProfile(profileParts{
			endpoint:          endpoint,
			title:             firstNonEmpty(strings.TrimSpace(tp.GetTitle()), endpoint, "VK TURN"),
			transportKind:     kind,
			links:             links,
			linkSecondary:     firstNonEmpty(strings.TrimSpace(tp.GetConfig().GetLinkSecondary()), strings.TrimSpace(turn.GetLinkSecondary())),
			settings:          s,
			wg:                resolveWG(cfg, tp.GetTransportProfileId()),
			managed:           tp.GetWgProvisioned(),
			provisionClientID: strings.TrimSpace(tp.GetProvisionClientId()),
			provisionToken:    base64.StdEncoding.EncodeToString(tp.GetProvisionToken()),
		}))
	}
	return out
}

type profileParts struct {
	endpoint          string
	title             string
	transportKind     string
	links             []string
	linkSecondary     string
	settings          Settings
	wg                WireGuard
	managed           bool
	provisionClientID string
	provisionToken    string
}

func buildProfile(p profileParts) Profile {
	kind := p.transportKind
	if kind == "" {
		kind = "wg"
	}
	return Profile{
		Title:             p.title,
		VKTurnEndpoint:    p.endpoint,
		TransportKind:     kind,
		Links:             p.links,
		LinkSecondary:     p.linkSecondary,
		Managed:           p.managed,
		ProvisionClientID: p.provisionClientID,
		ProvisionToken:    p.provisionToken,
		Settings:          p.settings,
		WG:                p.wg,
	}
}

// turnLinks merges the singular link and the links list, deduped, preserving order.
func turnLinks(turn *wingsvpb.Turn) []string {
	if turn == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(turn.GetLink())
	for _, l := range turn.GetLinks() {
		add(l)
	}
	return out
}

// overlayTurn applies the fields a Turn message actually sets onto s, leaving the
// rest at their (app-default) values. Optional scalars are gated on presence.
func overlayTurn(s *Settings, turn *wingsvpb.Turn) {
	if turn == nil {
		return
	}
	if turn.Threads != nil && turn.GetThreads() > 0 {
		s.Threads = int(turn.GetThreads())
	}
	if turn.CredsGroupSize != nil && turn.GetCredsGroupSize() > 0 {
		s.CredsGroupSize = int(turn.GetCredsGroupSize())
	}
	if turn.UseUdp != nil {
		s.UseUDP = turn.GetUseUdp()
	}
	if turn.NoObfuscation != nil {
		s.NoObfuscation = turn.GetNoObfuscation()
	}
	if turn.ManualCaptcha != nil {
		s.ManualCaptcha = turn.GetManualCaptcha()
	}
	if turn.RestartOnNetworkChange != nil {
		s.RestartOnNetworkChange = turn.GetRestartOnNetworkChange()
	}
	if v := strings.TrimSpace(turn.GetCaptchaAutoSolver()); v != "" {
		s.CaptchaAutoSolver = v
	}
	if m := sessionModeString(turn.GetSessionMode()); m != "" {
		s.TurnSessionMode = m
	}
	if m := runtimeModeString(turn.GetRuntimeMode()); m != "" {
		s.RuntimeMode = m
	}
	if m := wrapModeString(turn.GetWrapMode()); m != "" {
		s.WrapMode = m
	}
	if c := wrapCipherString(turn.GetWrapCiphers()); c != "" {
		s.WrapCipher = c
	}
	if len(turn.GetWrapKey()) > 0 {
		s.WrapKeyHex = hex.EncodeToString(turn.GetWrapKey())
	}
	switch turn.GetWrapKeyDelivery() {
	case wingsvpb.WrapKeyDelivery_WRAP_KEY_DELIVERY_IN_BAND:
		s.WrapSendKey = true
	case wingsvpb.WrapKeyDelivery_WRAP_KEY_DELIVERY_OFF:
		s.WrapSendKey = false
	}
	if dns := strings.TrimSpace(strings.Join(turn.GetUserDns(), "\n")); dns != "" {
		s.UserDNS = dns
	}
	if v := endpointString(turn.GetLocalEndpoint()); v != "" {
		s.LocalEndpoint = v
	}
	if v := strings.TrimSpace(turn.GetHost()); v != "" {
		s.TurnHost = v
	}
	if turn.Port != nil && turn.GetPort() > 0 {
		s.TurnPort = strconv.Itoa(int(turn.GetPort()))
	}
	if v := strings.TrimSpace(turn.GetBrowserFingerprint()); v != "" {
		s.BrowserFingerprint = v
	}
}

func resolveWG(cfg *wingsvpb.Config, transportProfileID string) WireGuard {
	id := strings.TrimSpace(transportProfileID)
	if id != "" {
		for _, wp := range cfg.GetWg().GetProfiles() {
			if wp.GetId() == id {
				return wgFromParts(wp.GetIface(), wp.GetPeer(), wp.GetEndpoint())
			}
		}
		// AmneziaWG transport profiles carry a raw awg-quick config blob.
		for _, ap := range cfg.GetAwg().GetProfiles() {
			if ap.GetId() == id {
				return parseAwgQuick(ap.GetAwgQuickConfig())
			}
		}
	}
	if raw := strings.TrimSpace(cfg.GetAwg().GetAwgQuickConfig()); raw != "" {
		return parseAwgQuick(raw)
	}
	return wgFromProto(cfg.GetWg())
}

// parseAwgQuick fills a WireGuard from a raw awg-quick config ([Interface]/[Peer],
// INI-style), including the AmneziaWG junk params. Unknown keys are ignored.
func parseAwgQuick(raw string) WireGuard {
	wg := DefaultWireGuard()
	section := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		switch section {
		case "interface":
			switch key {
			case "privatekey":
				wg.PrivateKey = val
			case "address":
				wg.Addresses = val
			case "dns":
				wg.DNS = val
			case "mtu":
				if n, err := strconv.Atoi(val); err == nil {
					wg.MTU = n
				}
			case "jc":
				wg.Jc = val
			case "jmin":
				wg.Jmin = val
			case "jmax":
				wg.Jmax = val
			case "s1":
				wg.S1 = val
			case "s2":
				wg.S2 = val
			case "s3":
				wg.S3 = val
			case "s4":
				wg.S4 = val
			case "h1":
				wg.H1 = val
			case "h2":
				wg.H2 = val
			case "h3":
				wg.H3 = val
			case "h4":
				wg.H4 = val
			}
		case "peer":
			switch key {
			case "publickey":
				wg.PublicKey = val
			case "presharedkey":
				wg.PresharedKey = val
			case "allowedips":
				wg.AllowedIPs = val
			case "endpoint":
				wg.Endpoint = val
			}
		}
	}
	return wg
}

// LooksLikeQuickConfig reports whether raw text is a WireGuard/AmneziaWG quick-config
// (an awg-quick/wg-quick INI) rather than a wingsv:// link: it must carry both an
// [Interface] and a [Peer] section.
func LooksLikeQuickConfig(raw string) bool {
	low := strings.ToLower(raw)
	return strings.Contains(low, "[interface]") && strings.Contains(low, "[peer]")
}

// ProfileFromQuickConfig builds a standalone profile from a raw awg-quick/wg-quick
// config. It is AmneziaWG when any junk parameter is present, plain WireGuard
// otherwise; the title defaults to the peer endpoint host.
func ProfileFromQuickConfig(raw string) Profile {
	wg := parseAwgQuick(raw)
	kind := "wg"
	if wg.Jc != "" || wg.Jmin != "" || wg.Jmax != "" ||
		wg.S1 != "" || wg.S2 != "" || wg.S3 != "" || wg.S4 != "" ||
		wg.H1 != "" || wg.H2 != "" || wg.H3 != "" || wg.H4 != "" {
		kind = "awg"
	}
	endpoint := strings.TrimSpace(wg.Endpoint)
	title := firstNonEmpty(strings.Split(endpoint, ":")[0], endpoint, "WireGuard")
	return Profile{
		Title:          title,
		VKTurnEndpoint: "",
		TransportKind:  kind,
		Settings:       DefaultSettings(),
		WG:             wg,
	}
}

func wgFromProto(wg *wingsvpb.WireGuard) WireGuard {
	if wg == nil {
		return DefaultWireGuard()
	}
	return wgFromParts(wg.GetIface(), wg.GetPeer(), wg.GetEndpoint())
}

func wgFromParts(iface *wingsvpb.Interface, peer *wingsvpb.Peer, endpoint *wingsvpb.Endpoint) WireGuard {
	wg := DefaultWireGuard()
	if iface != nil {
		if len(iface.GetPrivateKey()) > 0 {
			wg.PrivateKey = base64.StdEncoding.EncodeToString(iface.GetPrivateKey())
		}
		if addrs := strings.Join(iface.GetAddrs(), ", "); addrs != "" {
			wg.Addresses = addrs
		}
		if dns := strings.Join(iface.GetDns(), ", "); dns != "" {
			wg.DNS = dns
		}
		if iface.Mtu != nil && iface.GetMtu() > 0 {
			wg.MTU = int(iface.GetMtu())
		}
	}
	if peer != nil {
		if len(peer.GetPublicKey()) > 0 {
			wg.PublicKey = base64.StdEncoding.EncodeToString(peer.GetPublicKey())
		}
		if len(peer.GetPresharedKey()) > 0 {
			wg.PresharedKey = base64.StdEncoding.EncodeToString(peer.GetPresharedKey())
		}
		if cidrs := cidrList(peer.GetAllowedIps()); cidrs != "" {
			wg.AllowedIPs = cidrs
		}
	}
	if ep := endpointString(endpoint); ep != "" {
		wg.Endpoint = ep
	}
	return wg
}

func cidrList(cidrs []*wingsvpb.Cidr) string {
	parts := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		ip := net.IP(c.GetAddr())
		if ip == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s/%d", ip.String(), c.GetPrefix()))
	}
	return strings.Join(parts, ", ")
}

func endpointString(ep *wingsvpb.Endpoint) string {
	if ep == nil {
		return ""
	}
	return hostPort(ep.GetHost(), ep.GetPort())
}

func hostPort(host string, port uint32) string {
	host = strings.TrimSpace(host)
	if host == "" && port == 0 {
		return ""
	}
	if port == 0 {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func tunnelKind(mode wingsvpb.TunnelMode) string {
	if mode == wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG {
		return "awg"
	}
	return "wg"
}

func sessionModeString(mode wingsvpb.TurnSessionMode) string {
	switch mode {
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_AUTO:
		return "auto"
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MAINLINE:
		return "mainline"
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX:
		return "mu"
	default:
		return ""
	}
}

func runtimeModeString(mode wingsvpb.ProxyRuntimeMode) string {
	switch mode {
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_VPN:
		return "vpn"
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_PROXY:
		return "proxy"
	default:
		return ""
	}
}

func wrapModeString(mode wingsvpb.WrapMode) string {
	switch mode {
	case wingsvpb.WrapMode_WRAP_MODE_OFF:
		return "off"
	case wingsvpb.WrapMode_WRAP_MODE_PREFERRED:
		return "preferred"
	case wingsvpb.WrapMode_WRAP_MODE_REQUIRED:
		return "required"
	default:
		return ""
	}
}

func wrapCipherString(ciphers []wingsvpb.WrapCipher) string {
	for _, c := range ciphers {
		switch c {
		case wingsvpb.WrapCipher_WRAP_CIPHER_SRTP_AES_256_GCM:
			return "srtp-aes-gcm"
		case wingsvpb.WrapCipher_WRAP_CIPHER_SRTP_CHACHA20_POLY1305:
			return "srtp-chacha20-poly1305"
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
