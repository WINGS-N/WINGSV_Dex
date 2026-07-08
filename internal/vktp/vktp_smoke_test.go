package vktp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/gen/appcontrolpb"
)

// TestSmokeStartConfigure exercises the real child-process + AppControl IPC path:
// it launches bin/vkturn, connects the unix-socket gRPC, and applies Configure.
// It is gated behind WINGSV_DEX_SMOKE=1 (and a built binary) so it never runs in
// the normal suite or CI - it spawns a real process and boots the relay engine.
func TestSmokeStartConfigure(t *testing.T) {
	if os.Getenv("WINGSV_DEX_SMOKE") != "1" {
		t.Skip("set WINGSV_DEX_SMOKE=1 to run the vkturn IPC smoke test")
	}
	bin, err := filepath.Abs("../../bin/vkturn")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("vkturn binary not built at %s (run: task build:vkturn)", bin)
	}

	sock := filepath.Join(t.TempDir(), "appcontrol.sock")
	mgr := NewManager(bin, sock, "", os.Stderr)

	// A well-formed but non-routable/anonymous profile: TEST-NET peer and an
	// .invalid VK link so the engine never touches a real host.
	p := config.Profile{
		VKTurnEndpoint: "203.0.113.10:51820",
		TransportKind:  "wg",
		Settings:       config.DefaultSettings(),
	}
	p.Settings.LocalEndpoint = "127.0.0.1:19000"

	cs := config.DefaultClientSettings()
	cs.VKLinks = []string{"https://vk.invalid/call/join/smoke"}

	if err := mgr.Start(p, cs, nil); err != nil {
		t.Fatalf("Start (IPC round-trip failed): %v", err)
	}
	defer mgr.Stop()

	if !mgr.Running() {
		t.Fatal("manager should report running after Start")
	}
	t.Log("vkturn launched, AppControl gRPC connected, Configure applied")

	// The StreamEvents RPC must open and stream the relay's structured events
	// (caps at boot, etc.) over gRPC - the channel that replaces stdout scraping.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var events int
	if err := mgr.Events(ctx, func(ev *appcontrolpb.ProxyEvent) {
		events++
		if st := ev.GetStatus(); st != nil {
			t.Logf("event: status phase=%s streams=%d", st.GetPhase(), st.GetConnectedStreams())
		} else if c := ev.GetCaptcha(); c != nil {
			t.Logf("event: captcha state=%s", c.GetState())
		} else {
			t.Logf("event: %v", ev.GetEvent())
		}
	}); err != nil {
		t.Fatalf("Events stream (StreamEvents RPC) failed: %v", err)
	}
	t.Logf("StreamEvents delivered %d event(s) over gRPC", events)
}
