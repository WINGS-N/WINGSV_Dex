package config

import (
	"testing"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
)

func TestProfilesFromConfigDefaultsAndTransport(t *testing.T) {
	cfg := &wingsvpb.Config{
		Turn: &wingsvpb.Turn{
			Endpoint:   &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
			TunnelMode: wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD,
			Title:      "test-profile",
		},
		Wg: &wingsvpb.WireGuard{
			Iface:    &wingsvpb.Interface{Addrs: []string{"10.8.0.2/32"}},
			Peer:     &wingsvpb.Peer{AllowedIps: []*wingsvpb.Cidr{{Addr: []byte{0, 0, 0, 0}, Prefix: 0}}},
			Endpoint: &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
		},
	}
	profiles := ProfilesFromConfig(cfg)
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if p.VKTurnEndpoint != "203.0.113.10:51820" {
		t.Fatalf("endpoint = %q", p.VKTurnEndpoint)
	}
	if p.Title != "test-profile" {
		t.Fatalf("title = %q", p.Title)
	}
	if p.TransportKind != "wg" {
		t.Fatalf("transportKind = %q", p.TransportKind)
	}
	// Unset settings must fall back to the app defaults.
	if p.Settings.Threads != 24 {
		t.Errorf("threads = %d, want 24", p.Settings.Threads)
	}
	if p.Settings.TurnSessionMode != "mu" {
		t.Errorf("turnSessionMode = %q, want mu", p.Settings.TurnSessionMode)
	}
	if p.Settings.CaptchaAutoSolver != "v2" {
		t.Errorf("captchaAutoSolver = %q, want v2", p.Settings.CaptchaAutoSolver)
	}
	if p.Settings.LocalEndpoint != "127.0.0.1:9000" {
		t.Errorf("localEndpoint = %q, want 127.0.0.1:9000", p.Settings.LocalEndpoint)
	}
	// WireGuard transport resolves from the top-level wg.
	if p.WG.Addresses != "10.8.0.2/32" {
		t.Errorf("wg addresses = %q", p.WG.Addresses)
	}
	if p.WG.AllowedIPs != "0.0.0.0/0" {
		t.Errorf("wg allowedIps = %q", p.WG.AllowedIPs)
	}
	if p.WG.MTU != 1280 {
		t.Errorf("wg mtu = %d, want 1280", p.WG.MTU)
	}
}
