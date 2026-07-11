package config

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestParseShareLinkVless(t *testing.T) {
	link := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@v.wingsnet.org:443?encryption=none&security=tls#Cyprus%20Node"
	p, ok := ParseShareLink(link)
	if !ok {
		t.Fatalf("expected vless link to parse")
	}
	if p.Address != "v.wingsnet.org" {
		t.Errorf("address = %q, want v.wingsnet.org", p.Address)
	}
	if p.Port != 443 {
		t.Errorf("port = %d, want 443", p.Port)
	}
	if p.Title != "Cyprus Node" {
		t.Errorf("title = %q, want %q", p.Title, "Cyprus Node")
	}
	if p.RawLink != link {
		t.Errorf("raw link not preserved")
	}
}

func TestParseShareLinkVmess(t *testing.T) {
	body := `{"v":"2","ps":"vm-node","add":"v.wingsnet.org","port":"8443","id":"uuid"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(body))
	p, ok := ParseShareLink(link)
	if !ok {
		t.Fatalf("expected vmess link to parse")
	}
	if p.Address != "v.wingsnet.org" || p.Port != 8443 || p.Title != "vm-node" {
		t.Errorf("got addr=%q port=%d title=%q", p.Address, p.Port, p.Title)
	}
}

func TestParseShareLinkRejectsNonXray(t *testing.T) {
	if _, ok := ParseShareLink("https://v.wingsnet.org"); ok {
		t.Errorf("https should not parse as an xray link")
	}
	if _, ok := ParseShareLink("wingsv://abc"); ok {
		t.Errorf("wingsv should not parse as an xray link")
	}
}

func TestDefaultXraySettings(t *testing.T) {
	d := DefaultXraySettings()
	if !d.IPv6 || !d.SniffingEnabled {
		t.Errorf("ipv6/sniffing should default true")
	}
	if d.LocalProxyPort != 10808 || d.HTTPProxyPort != 10809 {
		t.Errorf("local/http ports wrong: %d/%d", d.LocalProxyPort, d.HTTPProxyPort)
	}
	if d.RemoteDNS != yandexDoH {
		t.Errorf("remote dns = %q", d.RemoteDNS)
	}
}

func TestEnsureCredsGeneratesOnce(t *testing.T) {
	x := DefaultXraySettings()
	if !x.ensureCreds() {
		t.Fatalf("expected creds to be generated")
	}
	if x.LocalProxyUsername == "" || x.LocalProxyPassword == "" {
		t.Errorf("local proxy creds not filled")
	}
	first := x.LocalProxyUsername
	if x.ensureCreds() {
		t.Errorf("creds should not regenerate once set")
	}
	if x.LocalProxyUsername != first {
		t.Errorf("creds changed on second ensure")
	}
}

func TestImportXraySwitchesBackend(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s.NetworkBackend() != BackendVKTurn {
		t.Fatalf("fresh store should default to vk_turn")
	}
	link := "vless://uuid@v.wingsnet.org:443?security=reality#Node1"
	profiles, err := s.ImportXray(link)
	if err != nil {
		t.Fatalf("ImportXray: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 xray profile, got %d", len(profiles))
	}
	if s.NetworkBackend() != BackendXray {
		t.Errorf("backend should switch to xray after import")
	}
	if s.XrayActiveID() != profiles[0].ID {
		t.Errorf("imported profile should become active")
	}
	// Re-import the same link dedupes rather than appending.
	profiles, err = s.ImportXray(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Errorf("re-import should dedupe, got %d", len(profiles))
	}
}
