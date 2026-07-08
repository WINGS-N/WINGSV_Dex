package config

import (
	"path/filepath"
	"testing"

	"github.com/WINGS-N/wingsv-dex/internal/gen/wingsvpb"
	"github.com/WINGS-N/wingsv-dex/internal/wingsv"
)

func sampleLink(t *testing.T) string {
	t.Helper()
	cfg := &wingsvpb.Config{
		Turn: &wingsvpb.Turn{
			Endpoint:   &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
			TunnelMode: wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD,
			Title:      "test-profile",
		},
		Wg: &wingsvpb.WireGuard{
			Iface:    &wingsvpb.Interface{Addrs: []string{"10.8.0.2/32"}},
			Peer:     &wingsvpb.Peer{PublicKey: make([]byte, 32)},
			Endpoint: &wingsvpb.Endpoint{Host: "203.0.113.10", Port: 51820},
		},
	}
	link, err := wingsv.Encode(cfg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return link
}

func TestStoreImportDedupFavoritePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	link := sampleLink(t)
	if _, err := store.Import(link); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got := store.List(); len(got) != 1 {
		t.Fatalf("after import want 1 profile, got %d", len(got))
	}
	id := store.List()[0].ID
	if id == "" {
		t.Fatal("profile id not assigned")
	}
	if store.ActiveID() != id {
		t.Fatalf("first import should activate; activeId=%q id=%q", store.ActiveID(), id)
	}

	// Re-importing the same server must dedup, keeping the same id.
	if _, err := store.Import(link); err != nil {
		t.Fatalf("re-Import: %v", err)
	}
	if got := store.List(); len(got) != 1 {
		t.Fatalf("after dedup want 1 profile, got %d", len(got))
	}
	if store.List()[0].ID != id {
		t.Fatalf("dedup changed id: %q -> %q", id, store.List()[0].ID)
	}

	if err := store.ToggleFavorite(id); err != nil {
		t.Fatalf("ToggleFavorite: %v", err)
	}

	// Reload from disk and confirm the favorite persisted.
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.List()
	if len(got) != 1 || !got[0].Favorite {
		t.Fatalf("favorite did not persist: %+v", got)
	}
	if reloaded.ActiveID() != id {
		t.Fatalf("active id did not persist: %q", reloaded.ActiveID())
	}

	if err := reloaded.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(reloaded.List()) != 0 || reloaded.ActiveID() != "" {
		t.Fatalf("remove left residue: profiles=%d active=%q", len(reloaded.List()), reloaded.ActiveID())
	}
}
