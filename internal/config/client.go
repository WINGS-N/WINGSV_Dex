package config

import "strings"

// ClientSettings are the device-global VK TURN parameters: they are shared across
// every profile and are never reset when the active profile changes. The VK-links
// pool is an append-only, order-preserving, deduped set; the primary link the relay
// uses is the first entry.
type ClientSettings struct {
	VKLinks                []string `json:"vkLinks"`
	VKLinkSecondary        string   `json:"vkLinkSecondary"`
	Threads                int      `json:"threads"`
	CredsGroupSize         int      `json:"credsGroupSize"`
	VKAuthMode             string   `json:"vkAuthMode"` // "anonymous" | "account"
	TurnSessionMode        string   `json:"turnSessionMode"`
	BrowserFingerprint     string   `json:"browserFingerprint"`
	RuntimeMode            string   `json:"runtimeMode"` // "vpn" | "proxy"
	RestartOnNetworkChange bool     `json:"restartOnNetworkChange"`
	LocalEndpoint          string   `json:"localEndpoint"`
}

// DefaultClientSettings returns the device-global defaults applied at runtime.
func DefaultClientSettings() ClientSettings {
	return ClientSettings{
		Threads:                24,
		CredsGroupSize:         12,
		VKAuthMode:             "anonymous",
		TurnSessionMode:        "mu",
		BrowserFingerprint:     "safari",
		RuntimeMode:            "vpn",
		RestartOnNetworkChange: true,
		LocalEndpoint:          "127.0.0.1:9000",
	}
}

// withDefaults fills any unset scalar with its default, so a store written before a
// field existed still yields sane runtime values.
func (c ClientSettings) withDefaults() ClientSettings {
	d := DefaultClientSettings()
	if c.Threads <= 0 {
		c.Threads = d.Threads
	}
	if c.CredsGroupSize <= 0 {
		c.CredsGroupSize = d.CredsGroupSize
	}
	if strings.TrimSpace(c.VKAuthMode) == "" {
		c.VKAuthMode = d.VKAuthMode
	}
	if strings.TrimSpace(c.TurnSessionMode) == "" {
		c.TurnSessionMode = d.TurnSessionMode
	}
	if strings.TrimSpace(c.BrowserFingerprint) == "" {
		c.BrowserFingerprint = d.BrowserFingerprint
	}
	if strings.TrimSpace(c.RuntimeMode) == "" {
		c.RuntimeMode = d.RuntimeMode
	}
	if strings.TrimSpace(c.LocalEndpoint) == "" {
		c.LocalEndpoint = d.LocalEndpoint
	}
	return c
}

// mergeVKLinks appends links to pool that are not already present, trimming blanks
// and preserving insertion order. The pool is never truncated (import is additive).
func mergeVKLinks(pool, links []string) []string {
	seen := make(map[string]bool, len(pool))
	out := make([]string, 0, len(pool)+len(links))
	for _, l := range pool {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	for _, l := range links {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}
