package config

import (
	"encoding/base64"
	"encoding/hex"
	"net"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

// ToConfig builds a wingsv Config for a single self-contained VK TURN profile, the
// reverse of ProfilesFromConfig. It is used to export the active profile as a
// wingsv:// share link. Fields that live only on the per-profile TurnProfile in the
// app (dns_mode, vk_auth_mode) are not carried on the top-level Turn and fall back
// to defaults on re-import.
func (p Profile) ToConfig() *wingsvpb.Config {
	s := p.Settings
	turn := &wingsvpb.Turn{
		Endpoint:               parseEndpoint(p.VKTurnEndpoint),
		Links:                  p.Links,
		LinkSecondary:          p.LinkSecondary,
		Threads:                proto.Uint32(uint32(s.Threads)),
		CredsGroupSize:         proto.Uint32(uint32(s.CredsGroupSize)),
		UseUdp:                 proto.Bool(s.UseUDP),
		NoObfuscation:          proto.Bool(s.NoObfuscation),
		ManualCaptcha:          proto.Bool(s.ManualCaptcha),
		CaptchaAutoSolver:      s.CaptchaAutoSolver,
		RestartOnNetworkChange: proto.Bool(s.RestartOnNetworkChange),
		SessionMode:            sessionModeEnum(s.TurnSessionMode),
		RuntimeMode:            runtimeModeEnum(s.RuntimeMode),
		TunnelMode:             tunnelModeEnum(p.TransportKind),
		WrapMode:               wrapModeEnum(s.WrapMode),
		WrapCiphers:            wrapCipherEnums(s.WrapCipher),
		WrapKey:                hexToBytes(s.WrapKeyHex),
		WrapKeyDelivery:        wrapDeliveryEnum(s.WrapSendKey),
		BrowserFingerprint:     s.BrowserFingerprint,
		Title:                  p.Title,
		LocalEndpoint:          parseEndpoint(s.LocalEndpoint),
		Host:                   s.TurnHost,
		UserDns:                splitList(s.UserDNS),
	}
	if port := parsePort(s.TurnPort); port > 0 {
		turn.Port = proto.Uint32(port)
	}
	// A managed profile stores no WG of its own - the node provisions it on connect - and
	// the provision handle (client id + token) lives on a TurnProfile, not the top-level
	// Turn. The single-profile decode path (ProfilesFromConfig with no profiles) never reads
	// those fields, so without this a copied managed profile re-imports as a plain, keyless
	// profile and WGUp fails with an empty private key. Emit a one-element profile list to
	// carry the provision handle across the share link.
	if p.Managed {
		turn.Profiles = []*wingsvpb.TurnProfile{{
			Title:             p.Title,
			VkTurnEndpoint:    p.VKTurnEndpoint,
			TransportKind:     p.TransportKind,
			WgProvisioned:     true,
			ProvisionClientId: p.ProvisionClientID,
			ProvisionToken:    base64ToBytes(p.ProvisionToken),
		}}
	}
	return &wingsvpb.Config{
		Ver:     1,
		Type:    wingsvpb.ConfigType_CONFIG_TYPE_VK_TURN_PROFILE,
		Backend: wingsvpb.BackendType_BACKEND_TYPE_VK_TURN,
		Turn:    turn,
		Wg:      wgToProto(p.WG),
	}
}

func wgToProto(wg WireGuard) *wingsvpb.WireGuard {
	iface := &wingsvpb.Interface{
		Addrs: splitCSV(wg.Addresses),
		Dns:   splitCSV(wg.DNS),
	}
	if key := base64ToBytes(wg.PrivateKey); len(key) > 0 {
		iface.PrivateKey = key
	}
	if wg.MTU > 0 {
		iface.Mtu = proto.Uint32(uint32(wg.MTU))
	}
	peer := &wingsvpb.Peer{
		PublicKey:  base64ToBytes(wg.PublicKey),
		AllowedIps: cidrsToProto(wg.AllowedIPs),
	}
	if psk := base64ToBytes(wg.PresharedKey); len(psk) > 0 {
		peer.PresharedKey = psk
	}
	return &wingsvpb.WireGuard{Iface: iface, Peer: peer, Endpoint: parseEndpoint(wg.Endpoint)}
}

func parseEndpoint(hostport string) *wingsvpb.Endpoint {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return nil
	}
	host := hostport
	var port uint32
	if i := strings.LastIndex(hostport, ":"); i >= 0 && !strings.Contains(hostport[i+1:], "]") {
		host = hostport[:i]
		port = parsePort(hostport[i+1:])
	}
	host = strings.Trim(host, "[]")
	return &wingsvpb.Endpoint{Host: host, Port: port}
}

func parsePort(s string) uint32 {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > 65535 {
		return 0
	}
	return uint32(n)
}

func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return b
}

func base64ToBytes(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return b
}

func cidrsToProto(s string) []*wingsvpb.Cidr {
	var out []*wingsvpb.Cidr
	for _, part := range splitCSV(s) {
		_, ipnet, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		prefix, _ := ipnet.Mask.Size()
		out = append(out, &wingsvpb.Cidr{Addr: ipnet.IP, Prefix: uint32(prefix)})
	}
	return out
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' }) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func sessionModeEnum(v string) wingsvpb.TurnSessionMode {
	switch v {
	case "auto":
		return wingsvpb.TurnSessionMode_TURN_SESSION_MODE_AUTO
	case "mainline":
		return wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MAINLINE
	case "mu":
		return wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX
	default:
		return wingsvpb.TurnSessionMode_TURN_SESSION_MODE_UNSPECIFIED
	}
}

func runtimeModeEnum(v string) wingsvpb.ProxyRuntimeMode {
	if v == "proxy" {
		return wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_PROXY
	}
	return wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_VPN
}

func tunnelModeEnum(kind string) wingsvpb.TunnelMode {
	if kind == "awg" {
		return wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG
	}
	return wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD
}

func wrapModeEnum(v string) wingsvpb.WrapMode {
	switch v {
	case "off":
		return wingsvpb.WrapMode_WRAP_MODE_OFF
	case "required":
		return wingsvpb.WrapMode_WRAP_MODE_REQUIRED
	default:
		return wingsvpb.WrapMode_WRAP_MODE_PREFERRED
	}
}

func wrapCipherEnums(v string) []wingsvpb.WrapCipher {
	switch v {
	case "srtp-chacha20-poly1305":
		return []wingsvpb.WrapCipher{wingsvpb.WrapCipher_WRAP_CIPHER_SRTP_CHACHA20_POLY1305}
	case "srtp-aes-gcm":
		return []wingsvpb.WrapCipher{wingsvpb.WrapCipher_WRAP_CIPHER_SRTP_AES_256_GCM}
	default:
		return nil
	}
}

func wrapDeliveryEnum(sendKey bool) wingsvpb.WrapKeyDelivery {
	if sendKey {
		return wingsvpb.WrapKeyDelivery_WRAP_KEY_DELIVERY_IN_BAND
	}
	return wingsvpb.WrapKeyDelivery_WRAP_KEY_DELIVERY_OFF
}
