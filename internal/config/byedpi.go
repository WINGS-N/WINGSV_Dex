package config

import (
	"strconv"
	"strings"
)

// ByeDPISettings configures the ByeDPI (ciadpi) local SOCKS proxy that xray can chain its
// outbound through as a DPI-bypass front. Fields and defaults follow the app's ByeDPI
// preferences.
type ByeDPISettings struct {
	Enabled        bool   `json:"enabled"`     // run ByeDPI and chain the xray outbound through it
	ProxyIP        string `json:"proxyIp"`     // local listen address
	ProxyPort      int    `json:"proxyPort"`   // local listen port
	AuthEnabled    bool   `json:"authEnabled"` // protect the local SOCKS with a login/password
	Username       string `json:"username"`
	Password       string `json:"password"`
	MaxConnections int    `json:"maxConnections"`
	BufferSize     int    `json:"bufferSize"`
	NoDomain       bool   `json:"noDomain"`
	TCPFastOpen    bool   `json:"tcpFastOpen"`
	DefaultTTL     int    `json:"defaultTtl"`
	DesyncMethod   string `json:"desyncMethod"` // oob | split | disorder | disoob | fake | auto
	SplitPosition  int    `json:"splitPosition"`
	FakeTTL        int    `json:"fakeTtl"`

	// UseCommandSettings replaces the structured flags above with a raw argv line, for
	// power users who want to pass ciadpi options verbatim.
	UseCommandSettings bool   `json:"useCommandSettings"`
	Command            string `json:"command"`
}

// DefaultByeDPISettings returns the ByeDPI defaults.
func DefaultByeDPISettings() ByeDPISettings {
	return ByeDPISettings{
		ProxyIP:        "127.0.0.1",
		ProxyPort:      1080,
		AuthEnabled:    true,
		MaxConnections: 512,
		BufferSize:     16384,
		DefaultTTL:     0,
		DesyncMethod:   "oob",
		SplitPosition:  1,
		FakeTTL:        8,
	}
}

// Normalized returns the settings with scalar defaults backstopped (used by the runner).
func (b ByeDPISettings) Normalized() ByeDPISettings { return b.withDefaults() }

func (b ByeDPISettings) withDefaults() ByeDPISettings {
	d := DefaultByeDPISettings()
	if b.ProxyIP == "" {
		b.ProxyIP = d.ProxyIP
	}
	if b.ProxyPort == 0 {
		b.ProxyPort = d.ProxyPort
	}
	if b.MaxConnections == 0 {
		b.MaxConnections = d.MaxConnections
	}
	if b.BufferSize == 0 {
		b.BufferSize = d.BufferSize
	}
	if b.DesyncMethod == "" {
		b.DesyncMethod = d.DesyncMethod
	}
	if b.SplitPosition == 0 {
		b.SplitPosition = d.SplitPosition
	}
	if b.FakeTTL == 0 {
		b.FakeTTL = d.FakeTTL
	}
	return b
}

// ensureCreds generates a login/password once when auth is enabled, like the xray local
// proxy. Returns true if anything was generated.
func (b *ByeDPISettings) ensureCreds() bool {
	if !b.AuthEnabled {
		return false
	}
	changed := false
	if b.Username == "" {
		b.Username = randomToken()
		changed = true
	}
	if b.Password == "" {
		b.Password = randomToken()
		changed = true
	}
	return changed
}

// ListenAddr is the host:port the ByeDPI SOCKS listens on.
func (b ByeDPISettings) ListenAddr() string {
	ip := strings.TrimSpace(b.ProxyIP)
	if ip == "" {
		ip = "127.0.0.1"
	}
	port := b.ProxyPort
	if port == 0 {
		port = 1080
	}
	return ip + ":" + strconv.Itoa(port)
}
