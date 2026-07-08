package config

// AppRoutingSettings is the per-app split-tunnel configuration. Following the Android
// model, a single mode selects which of two app-id lists is active: the bypass list
// (apps that go direct, out of the tunnel) or the whitelist (the only apps that enter
// the tunnel). App ids are executable basenames, matched against a process's comm/exe
// on Linux. The X-family modes (gVisor divert) are Android-only and not ported.
type AppRoutingSettings struct {
	Mode      string   `json:"mode"` // "off" | "bypass" | "whitelist"
	Bypass    []string `json:"bypass"`
	Whitelist []string `json:"whitelist"`
}

// withDefaults normalizes the mode to a known value (default "off": every app is
// tunneled, only the service protects bypass).
func (a AppRoutingSettings) withDefaults() AppRoutingSettings {
	if a.Mode != "bypass" && a.Mode != "whitelist" {
		a.Mode = "off"
	}
	if a.Bypass == nil {
		a.Bypass = []string{}
	}
	if a.Whitelist == nil {
		a.Whitelist = []string{}
	}
	return a
}

// ActiveList returns the app-id list the current mode applies (empty for "off").
func (a AppRoutingSettings) ActiveList() []string {
	switch a.Mode {
	case "bypass":
		return a.Bypass
	case "whitelist":
		return a.Whitelist
	default:
		return nil
	}
}
