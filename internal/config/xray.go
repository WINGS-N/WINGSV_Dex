package config

import (
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

// Network backend axis, orthogonal to the VK TURN sub-backend (wg|awg). Persisted on the
// store; selects whether the VK TURN or the Xray profile set is the active one.
const (
	BackendVKTurn = "vk_turn"
	BackendXray   = "xray"
)

// XraySettings is the flat Xray configuration. The per-app UID filter fields have no
// desktop equivalent (they are tied to VpnService/ConnectivityManager) and are omitted.
type XraySettings struct {
	AllowLan               bool   `json:"allowLan"`
	AllowInsecure          bool   `json:"allowInsecure"`
	IPv6                   bool   `json:"ipv6"`
	SniffingEnabled        bool   `json:"sniffingEnabled"`
	ProxyQuicEnabled       bool   `json:"proxyQuicEnabled"`
	RestartOnNetworkChange bool   `json:"restartOnNetworkChange"`
	RuntimeMode            string `json:"runtimeMode"`   // "vpn" | "proxy"
	TransportMode          string `json:"transportMode"` // "direct" | "vk_turn_tcp"
	RemoteDNS              string `json:"remoteDns"`
	DirectDNS              string `json:"directDns"`

	LocalProxyEnabled       bool   `json:"localProxyEnabled"`
	LocalProxyPort          int    `json:"localProxyPort"`
	LocalProxyListenAddress string `json:"localProxyListenAddress"`
	LocalProxyAuthEnabled   bool   `json:"localProxyAuthEnabled"`
	LocalProxyUsername      string `json:"localProxyUsername"`
	LocalProxyPassword      string `json:"localProxyPassword"`

	HTTPProxyEnabled       bool   `json:"httpProxyEnabled"`
	HTTPProxyPort          int    `json:"httpProxyPort"`
	HTTPProxyListenAddress string `json:"httpProxyListenAddress"`
	HTTPProxyAuthEnabled   bool   `json:"httpProxyAuthEnabled"`
	HTTPProxyUsername      string `json:"httpProxyUsername"`
	HTTPProxyPassword      string `json:"httpProxyPassword"`
}

const yandexDoH = "https://common.dot.dns.yandex.net/dns-query"

// DefaultXraySettings returns the Xray defaults.
func DefaultXraySettings() XraySettings {
	return XraySettings{
		IPv6:                    true,
		SniffingEnabled:         true,
		RuntimeMode:             "vpn",
		TransportMode:           "direct",
		RemoteDNS:               yandexDoH,
		DirectDNS:               yandexDoH,
		LocalProxyPort:          10808,
		LocalProxyListenAddress: "127.0.0.1",
		LocalProxyAuthEnabled:   true,
		HTTPProxyPort:           10809,
		HTTPProxyListenAddress:  "127.0.0.1",
		HTTPProxyAuthEnabled:    true,
	}
}

// withDefaults backstops scalar fields left empty by an older store file. Bools are not
// backstopped (a stored false is indistinguishable from unset), which is why a fresh store
// seeds DefaultXraySettings wholesale instead of relying on this.
func (x XraySettings) withDefaults() XraySettings {
	d := DefaultXraySettings()
	if x.RuntimeMode == "" {
		x.RuntimeMode = d.RuntimeMode
	}
	if x.TransportMode == "" {
		x.TransportMode = d.TransportMode
	}
	if x.RemoteDNS == "" {
		x.RemoteDNS = d.RemoteDNS
	}
	if x.DirectDNS == "" {
		x.DirectDNS = d.DirectDNS
	}
	if x.LocalProxyPort == 0 {
		x.LocalProxyPort = d.LocalProxyPort
	}
	if x.LocalProxyListenAddress == "" {
		x.LocalProxyListenAddress = d.LocalProxyListenAddress
	}
	if x.HTTPProxyPort == 0 {
		x.HTTPProxyPort = d.HTTPProxyPort
	}
	if x.HTTPProxyListenAddress == "" {
		x.HTTPProxyListenAddress = d.HTTPProxyListenAddress
	}
	return x
}

// ensureCreds fills auto-generated socks/http credentials once: a URL-safe base64 of 32
// random bytes, generated on first use and then persisted. Returns true if anything was
// generated (so the store can save).
func (x *XraySettings) ensureCreds() bool {
	changed := false
	if x.LocalProxyAuthEnabled {
		if x.LocalProxyUsername == "" {
			x.LocalProxyUsername = randomToken()
			changed = true
		}
		if x.LocalProxyPassword == "" {
			x.LocalProxyPassword = randomToken()
			changed = true
		}
	}
	if x.HTTPProxyAuthEnabled {
		if x.HTTPProxyUsername == "" {
			x.HTTPProxyUsername = randomToken()
			changed = true
		}
		if x.HTTPProxyPassword == "" {
			x.HTTPProxyPassword = randomToken()
			changed = true
		}
	}
	return changed
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "wingsv"
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// XrayProfile is a single VLESS/xray node. It stores the raw share link and only thin
// metadata; the actual xray outbound JSON is produced at connect time by the bin/xray
// convert helper (libXray).
type XrayProfile struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	RawLink           string `json:"rawLink"`
	SubscriptionID    string `json:"subscriptionId,omitempty"`
	SubscriptionTitle string `json:"subscriptionTitle,omitempty"`
	Address           string `json:"address"`
	Port              int    `json:"port"`
	Favorite          bool   `json:"favorite"`
}

// DedupKey identifies the same node across re-imports by its raw share link.
func (p XrayProfile) DedupKey() string { return strings.TrimSpace(p.RawLink) }

// Subscription is a remote list of xray nodes refreshed on a schedule.
type Subscription struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	URL                    string `json:"url"`
	RefreshIntervalMinutes int    `json:"refreshIntervalMinutes,omitempty"`
	AutoUpdate             bool   `json:"autoUpdate"`
	LastUpdatedAt          int64  `json:"lastUpdatedAt,omitempty"`
}

// xrayShareSchemes are the share-link schemes ParseShareLink recognizes as xray nodes.
var xrayShareSchemes = []string{"vless", "vmess", "trojan", "ss", "hysteria2", "hy2"}

// LooksLikeXrayLink reports whether a pasted string is an xray share link.
func LooksLikeXrayLink(raw string) bool {
	l := strings.ToLower(strings.TrimSpace(raw))
	for _, s := range xrayShareSchemes {
		if strings.HasPrefix(l, s+"://") {
			return true
		}
	}
	return false
}

// ParseShareLink extracts thin metadata (address, port, title) from an xray share link.
// vmess:// carries a base64 JSON body; the others use a userinfo@host:port authority.
// Returns false when the input is not a recognized xray link.
func ParseShareLink(raw string) (XrayProfile, bool) {
	raw = strings.TrimSpace(raw)
	if !LooksLikeXrayLink(raw) {
		return XrayProfile{}, false
	}
	p := XrayProfile{RawLink: raw}
	scheme := strings.ToLower(raw[:strings.Index(raw, "://")])
	if scheme == "vmess" {
		fillVmessMeta(&p, raw)
	} else {
		fillAuthorityMeta(&p, raw)
	}
	if p.Title == "" {
		p.Title = firstNonEmpty(p.Address, "Xray")
	}
	return p, true
}

func fillAuthorityMeta(p *XrayProfile, raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	p.Address = u.Hostname()
	if port, err := strconv.Atoi(u.Port()); err == nil {
		p.Port = port
	}
	if frag := strings.TrimSpace(u.Fragment); frag != "" {
		p.Title = frag
	}
}

func fillVmessMeta(p *XrayProfile, raw string) {
	body := strings.TrimSpace(raw[len("vmess://"):])
	if i := strings.IndexAny(body, "#?"); i >= 0 {
		body = body[:i]
	}
	decoded, err := decodeBase64Loose(body)
	if err != nil {
		return
	}
	// Minimal field pull without a full json model: add/port/ps.
	p.Address = jsonStringField(decoded, "add")
	if port := jsonStringField(decoded, "port"); port != "" {
		p.Port, _ = strconv.Atoi(strings.Trim(port, `"`))
	}
	p.Title = jsonStringField(decoded, "ps")
}

func decodeBase64Loose(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", errInvalidBase64
}

var errInvalidBase64 = &parseError{"invalid base64"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

// jsonStringField pulls a top-level "key":value pair out of a small flat JSON object
// without unmarshaling into a struct; value may be quoted or a bare number.
func jsonStringField(js, key string) string {
	i := strings.Index(js, `"`+key+`"`)
	if i < 0 {
		return ""
	}
	rest := js[i+len(key)+2:]
	c := strings.IndexByte(rest, ':')
	if c < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[c+1:])
	if rest == "" {
		return ""
	}
	if rest[0] == '"' {
		if end := strings.IndexByte(rest[1:], '"'); end >= 0 {
			return rest[1 : 1+end]
		}
		return ""
	}
	end := strings.IndexAny(rest, ",}")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// XrayProfilesFromConfig extracts the VLESS profiles carried by a decoded Config.
func XrayProfilesFromConfig(cfg *wingsvpb.Config) []XrayProfile {
	xr := cfg.GetXray()
	if xr == nil {
		return nil
	}
	out := make([]XrayProfile, 0, len(xr.GetProfiles()))
	for _, vp := range xr.GetProfiles() {
		out = append(out, XrayProfile{
			ID:                strings.TrimSpace(vp.GetId()),
			Title:             firstNonEmpty(strings.TrimSpace(vp.GetTitle()), vp.GetAddress(), "Xray"),
			RawLink:           strings.TrimSpace(vp.GetRawLink()),
			SubscriptionID:    strings.TrimSpace(vp.GetSubscriptionId()),
			SubscriptionTitle: strings.TrimSpace(vp.GetSubscriptionTitle()),
			Address:           strings.TrimSpace(vp.GetAddress()),
			Port:              int(vp.GetPort()),
		})
	}
	return out
}

// SubscriptionsFromConfig extracts the subscriptions carried by a decoded Config.
func SubscriptionsFromConfig(cfg *wingsvpb.Config) []Subscription {
	xr := cfg.GetXray()
	if xr == nil {
		return nil
	}
	out := make([]Subscription, 0, len(xr.GetSubscriptions()))
	for _, s := range xr.GetSubscriptions() {
		out = append(out, Subscription{
			ID:                     strings.TrimSpace(s.GetId()),
			Title:                  strings.TrimSpace(s.GetTitle()),
			URL:                    strings.TrimSpace(s.GetUrl()),
			RefreshIntervalMinutes: int(s.GetRefreshIntervalMinutes()),
			AutoUpdate:             s.GetAutoUpdate(),
			LastUpdatedAt:          s.GetLastUpdatedAt(),
		})
	}
	return out
}

// XraySettingsFromProto overlays a proto XraySettings onto the defaults, so absent
// fields keep their default value.
func XraySettingsFromProto(s *wingsvpb.XraySettings) XraySettings {
	x := DefaultXraySettings()
	if s == nil {
		return x
	}
	if s.AllowLan != nil {
		x.AllowLan = s.GetAllowLan()
	}
	if s.AllowInsecure != nil {
		x.AllowInsecure = s.GetAllowInsecure()
	}
	if s.Ipv6 != nil {
		x.IPv6 = s.GetIpv6()
	}
	if s.SniffingEnabled != nil {
		x.SniffingEnabled = s.GetSniffingEnabled()
	}
	if s.ProxyQuicEnabled != nil {
		x.ProxyQuicEnabled = s.GetProxyQuicEnabled()
	}
	if s.RestartOnNetworkChange != nil {
		x.RestartOnNetworkChange = s.GetRestartOnNetworkChange()
	}
	if m := xrayRuntimeModeString(s.GetRuntimeMode()); m != "" {
		x.RuntimeMode = m
	}
	if m := xrayTransportModeString(s.GetTransportMode()); m != "" {
		x.TransportMode = m
	}
	if v := strings.TrimSpace(s.GetRemoteDns()); v != "" {
		x.RemoteDNS = v
	}
	if v := strings.TrimSpace(s.GetDirectDns()); v != "" {
		x.DirectDNS = v
	}
	if s.LocalProxyEnabled != nil {
		x.LocalProxyEnabled = s.GetLocalProxyEnabled()
	}
	if s.LocalProxyPort != nil {
		x.LocalProxyPort = int(s.GetLocalProxyPort())
	}
	if v := strings.TrimSpace(s.GetLocalProxyListenAddress()); v != "" {
		x.LocalProxyListenAddress = v
	}
	if s.LocalProxyAuthEnabled != nil {
		x.LocalProxyAuthEnabled = s.GetLocalProxyAuthEnabled()
	}
	if v := s.GetLocalProxyUsername(); v != "" {
		x.LocalProxyUsername = v
	}
	if v := s.GetLocalProxyPassword(); v != "" {
		x.LocalProxyPassword = v
	}
	if s.HttpProxyEnabled != nil {
		x.HTTPProxyEnabled = s.GetHttpProxyEnabled()
	}
	if s.HttpProxyPort != nil {
		x.HTTPProxyPort = int(s.GetHttpProxyPort())
	}
	if v := strings.TrimSpace(s.GetHttpProxyListenAddress()); v != "" {
		x.HTTPProxyListenAddress = v
	}
	if s.HttpProxyAuthEnabled != nil {
		x.HTTPProxyAuthEnabled = s.GetHttpProxyAuthEnabled()
	}
	if v := s.GetHttpProxyUsername(); v != "" {
		x.HTTPProxyUsername = v
	}
	if v := s.GetHttpProxyPassword(); v != "" {
		x.HTTPProxyPassword = v
	}
	return x
}

func xrayRuntimeModeString(m wingsvpb.ProxyRuntimeMode) string {
	switch m {
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_PROXY:
		return "proxy"
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_VPN:
		return "vpn"
	default:
		return ""
	}
}

func xrayTransportModeString(m wingsvpb.XrayTransportMode) string {
	switch m {
	case wingsvpb.XrayTransportMode_XRAY_TRANSPORT_MODE_VK_TURN_TCP:
		return "vk_turn_tcp"
	case wingsvpb.XrayTransportMode_XRAY_TRANSPORT_MODE_DIRECT:
		return "direct"
	default:
		return ""
	}
}
