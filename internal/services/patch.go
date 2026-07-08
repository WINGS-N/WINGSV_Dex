package services

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/gen/appcontrolpb"
)

func strp(s string) *string { return &s }
func i32p(i int32) *int32   { return &i }
func boolp(b bool) *bool    { return &b }

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func linksEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// needsReconnect reports whether the change from the applied settings to the current
// ones requires a full relay restart - the WG transport, a provisioned endpoint /
// node identity, or a runtime field the relay cannot live-patch (user_dns,
// creds_group_size, session mode, browser fingerprint, local endpoint, runtime mode,
// secondary VK link, and the per-profile toggles). Everything the relay can live-apply
// is handled by buildPatchDelta instead.
func needsReconnect(oldP config.Profile, oldC config.ClientSettings, newP config.Profile, newC config.ClientSettings) bool {
	np := newP
	// WG transport / sub-backend change.
	if oldP.TransportKind != np.TransportKind || oldP.WG != np.WG {
		return true
	}
	// Provisioned endpoint / node identity is baked into the tunnel.
	if np.Managed != oldP.Managed || np.ProvisionClientID != oldP.ProvisionClientID || np.ProvisionToken != oldP.ProvisionToken {
		return true
	}
	if np.Managed && oldP.VKTurnEndpoint != np.VKTurnEndpoint {
		return true
	}
	os, ns := oldP.Settings, np.Settings
	if os.UserDNS != ns.UserDNS || os.UseUDP != ns.UseUDP || os.NoObfuscation != ns.NoObfuscation ||
		os.ManualCaptcha != ns.ManualCaptcha || os.CaptchaAutoSolver != ns.CaptchaAutoSolver {
		return true
	}
	if oldC.CredsGroupSize != newC.CredsGroupSize || oldC.TurnSessionMode != newC.TurnSessionMode ||
		oldC.BrowserFingerprint != newC.BrowserFingerprint || oldC.LocalEndpoint != newC.LocalEndpoint ||
		oldC.RuntimeMode != newC.RuntimeMode || oldC.VKLinkSecondary != newC.VKLinkSecondary {
		return true
	}
	return false
}

// buildPatchDelta returns a PatchConfigRequest carrying only the live-patchable
// fields that changed (DNS mode, endpoint for manual WG, TURN host/port, WRAP group,
// VK auth, thread count, VK links), or nil when nothing live-patchable changed.
func buildPatchDelta(oldP config.Profile, oldC config.ClientSettings, newP config.Profile, newC config.ClientSettings) *appcontrolpb.PatchConfigRequest {
	req := &appcontrolpb.PatchConfigRequest{RequestId: newRequestID()}
	changed := false
	os, ns := oldP.Settings, newP.Settings

	if os.DNSMode != ns.DNSMode {
		req.DnsMode = strp(ns.DNSMode)
		changed = true
	}
	if os.TurnHost != ns.TurnHost {
		req.TurnHost = strp(ns.TurnHost)
		changed = true
	}
	if os.TurnPort != ns.TurnPort {
		req.TurnPort = strp(ns.TurnPort)
		changed = true
	}
	// Endpoint is live only for manual WG (a provisioned change hits needsReconnect).
	if !newP.Managed && oldP.VKTurnEndpoint != newP.VKTurnEndpoint {
		req.Peer = strp(newP.VKTurnEndpoint)
		changed = true
	}
	// WRAP is a group: any change sends the whole set so the relay migrates it as one.
	if os.WrapMode != ns.WrapMode || os.WrapCipher != ns.WrapCipher || os.WrapKeyHex != ns.WrapKeyHex || os.WrapSendKey != ns.WrapSendKey {
		req.WrapMode = strp(ns.WrapMode)
		req.WrapCipher = strp(ns.WrapCipher)
		req.WrapKeyHex = strp(ns.WrapKeyHex)
		req.WrapSendKey = boolp(ns.WrapSendKey)
		changed = true
	}
	if oldC.VKAuthMode != newC.VKAuthMode {
		req.VkAuth = strp(newC.VKAuthMode)
		changed = true
	}
	if oldC.Threads != newC.Threads {
		req.Threads = i32p(int32(newC.Threads))
		changed = true
	}
	if !linksEqual(oldC.VKLinks, newC.VKLinks) {
		req.VkLinks = &appcontrolpb.VKLinksPatch{Links: newC.VKLinks}
		changed = true
	}
	if !changed {
		return nil
	}
	return req
}
