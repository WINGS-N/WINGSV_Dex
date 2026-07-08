package config

import "testing"

func TestProfileToConfigRoundTrip(t *testing.T) {
	orig := Profile{
		Title:          "test-profile",
		VKTurnEndpoint: "203.0.113.10:51820",
		TransportKind:  "wg",
		Links:          []string{"https://vk.invalid/call/join/a", "https://vk.invalid/call/join/b"},
		Settings: Settings{
			Threads:            8,
			CredsGroupSize:     4,
			UseUDP:             false,
			NoObfuscation:      true,
			CaptchaAutoSolver:  "v1",
			TurnSessionMode:    "mainline",
			RuntimeMode:        "proxy",
			WrapMode:           "required",
			WrapCipher:         "srtp-chacha20-poly1305",
			WrapSendKey:        false,
			BrowserFingerprint: "chrome",
			LocalEndpoint:      "127.0.0.1:9000",
		},
		WG: WireGuard{
			Addresses:  "10.8.0.2/32",
			DNS:        "1.1.1.1",
			MTU:        1420,
			AllowedIPs: "0.0.0.0/0",
			Endpoint:   "203.0.113.10:51820",
		},
	}

	back := ProfilesFromConfig(orig.ToConfig())
	if len(back) != 1 {
		t.Fatalf("want 1 profile, got %d", len(back))
	}
	g := back[0]
	if g.VKTurnEndpoint != orig.VKTurnEndpoint {
		t.Errorf("endpoint: %q != %q", g.VKTurnEndpoint, orig.VKTurnEndpoint)
	}
	if len(g.Links) != 2 {
		t.Errorf("links = %v", g.Links)
	}
	if g.Settings.Threads != 8 || g.Settings.CredsGroupSize != 4 {
		t.Errorf("threads/creds = %d/%d", g.Settings.Threads, g.Settings.CredsGroupSize)
	}
	if g.Settings.UseUDP != false || g.Settings.NoObfuscation != true {
		t.Errorf("udp/noobf = %v/%v", g.Settings.UseUDP, g.Settings.NoObfuscation)
	}
	if g.Settings.TurnSessionMode != "mainline" || g.Settings.RuntimeMode != "proxy" {
		t.Errorf("session/runtime = %q/%q", g.Settings.TurnSessionMode, g.Settings.RuntimeMode)
	}
	if g.Settings.WrapMode != "required" || g.Settings.WrapCipher != "srtp-chacha20-poly1305" || g.Settings.WrapSendKey != false {
		t.Errorf("wrap = %q/%q/%v", g.Settings.WrapMode, g.Settings.WrapCipher, g.Settings.WrapSendKey)
	}
	if g.Settings.BrowserFingerprint != "chrome" {
		t.Errorf("browserFp = %q", g.Settings.BrowserFingerprint)
	}
	if g.WG.Addresses != "10.8.0.2/32" || g.WG.MTU != 1420 || g.WG.AllowedIPs != "0.0.0.0/0" {
		t.Errorf("wg = %q/%d/%q", g.WG.Addresses, g.WG.MTU, g.WG.AllowedIPs)
	}
}

// A managed profile carries no WG of its own; the share link must round-trip the provision
// handle so the copy re-imports as managed (else WGUp fails with an empty private key).
func TestManagedProfileToConfigRoundTrip(t *testing.T) {
	orig := Profile{
		Title:             "managed-node",
		VKTurnEndpoint:    "max.whsrv.ru:56000",
		TransportKind:     "wg",
		Managed:           true,
		ProvisionClientID: "client-123",
		ProvisionToken:    "dG9rZW4tYnl0ZXM=", // base64("token-bytes")
		Settings:          DefaultSettings(),
	}

	back := ProfilesFromConfig(orig.ToConfig())
	if len(back) != 1 {
		t.Fatalf("want 1 profile, got %d", len(back))
	}
	g := back[0]
	if !g.Managed {
		t.Errorf("managed flag lost on round-trip")
	}
	if g.ProvisionClientID != "client-123" {
		t.Errorf("provisionClientId = %q", g.ProvisionClientID)
	}
	if g.ProvisionToken != "dG9rZW4tYnl0ZXM=" {
		t.Errorf("provisionToken = %q", g.ProvisionToken)
	}
	if g.VKTurnEndpoint != "max.whsrv.ru:56000" {
		t.Errorf("endpoint = %q", g.VKTurnEndpoint)
	}
}
