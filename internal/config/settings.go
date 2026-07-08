// Package config holds the VK TURN settings model and the profile store. The runtime
// default for turnSessionMode is "mu", not the "mainline" preferences default.
package config

// Settings is the flat VK TURN configuration.
type Settings struct {
	Threads                int    `json:"threads"`
	CredsGroupSize         int    `json:"credsGroupSize"`
	UseUDP                 bool   `json:"useUdp"`
	NoObfuscation          bool   `json:"noObfuscation"`
	ManualCaptcha          bool   `json:"manualCaptcha"`
	CaptchaAutoSolver      string `json:"captchaAutoSolver"`
	VKAuthMode             string `json:"vkAuthMode"` // "anonymous" | "account"
	TurnSessionMode        string `json:"turnSessionMode"`
	DNSMode                string `json:"dnsMode"`
	UserDNS                string `json:"userDns"`
	RuntimeMode            string `json:"runtimeMode"` // "vpn" | "proxy"
	RestartOnNetworkChange bool   `json:"restartOnNetworkChange"`
	WrapMode               string `json:"wrapMode"`
	WrapCipher             string `json:"wrapCipher"`
	WrapKeyHex             string `json:"wrapKeyHex"`
	WrapSendKey            bool   `json:"wrapSendKey"`
	LocalEndpoint          string `json:"localEndpoint"`
	TurnHost               string `json:"turnHost"`
	TurnPort               string `json:"turnPort"`
	BrowserFingerprint     string `json:"browserFingerprint"`
}

// DefaultSettings returns the VK TURN defaults applied at runtime.
func DefaultSettings() Settings {
	return Settings{
		Threads:                24,
		CredsGroupSize:         12,
		UseUDP:                 true,
		NoObfuscation:          false,
		ManualCaptcha:          false,
		CaptchaAutoSolver:      "v2",
		VKAuthMode:             "anonymous",
		TurnSessionMode:        "mu",
		DNSMode:                "auto",
		UserDNS:                "",
		RuntimeMode:            "vpn",
		RestartOnNetworkChange: true,
		WrapMode:               "preferred",
		WrapCipher:             "srtp-aes-gcm",
		WrapKeyHex:             "",
		WrapSendKey:            true,
		LocalEndpoint:          "127.0.0.1:9000",
		TurnHost:               "",
		TurnPort:               "",
		BrowserFingerprint:     "safari",
	}
}

// WireGuard is a resolved WireGuard transport for a VK TURN profile.
type WireGuard struct {
	PrivateKey   string `json:"privateKey"`
	Addresses    string `json:"addresses"`
	DNS          string `json:"dns"`
	MTU          int    `json:"mtu"`
	PublicKey    string `json:"publicKey"`
	PresharedKey string `json:"presharedKey"`
	AllowedIPs   string `json:"allowedIps"`
	Endpoint     string `json:"endpoint"`

	// AmneziaWG obfuscation params (empty for plain WireGuard, kept as strings):
	// Jc/Jmin/Jmax = junk packet count/min/max; S1..S4 = init/response/cookie/transport
	// junk sizes; H1..H4 = init/response/underload/transport magic headers.
	Jc   string `json:"jc"`
	Jmin string `json:"jmin"`
	Jmax string `json:"jmax"`
	S1   string `json:"s1"`
	S2   string `json:"s2"`
	S3   string `json:"s3"`
	S4   string `json:"s4"`
	H1   string `json:"h1"`
	H2   string `json:"h2"`
	H3   string `json:"h3"`
	H4   string `json:"h4"`
}

// DefaultWireGuard returns the WireGuard transport defaults.
func DefaultWireGuard() WireGuard {
	return WireGuard{
		DNS:        "1.1.1.1, 1.0.0.1",
		MTU:        1280,
		AllowedIPs: "0.0.0.0/0, ::/0",
	}
}
