package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/WINGS-N/wingsv-dex/internal/wingsv"
)

// Store persists VK TURN profiles to a JSON file. It is safe for concurrent use;
// the Wails bound service calls it from arbitrary frontend-invoked goroutines.
type Store struct {
	mu   sync.Mutex
	path string
	data storeData
}

type storeData struct {
	Profiles   []Profile          `json:"profiles"`
	ActiveID   string             `json:"activeId"`
	SubBackend string             `json:"subBackend"` // "wg" | "awg" global mode
	Client     ClientSettings     `json:"client"`     // device-global VK TURN params + VK-links pool
	AppRouting AppRoutingSettings `json:"appRouting"` // per-app split-tunnel mode + lists
}

// NewStore loads the store from path, creating an empty one if the file is absent.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Fresh store: seed the device-global defaults so bool-default fields (e.g.
		// restart-on-network-change) start true rather than at their zero value.
		s.data.Client = DefaultClientSettings()
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// List returns a copy of the stored profiles.
func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Profile(nil), s.data.Profiles...)
}

// ActiveID returns the id of the active profile, or empty if none.
func (s *Store) ActiveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.ActiveID
}

// SubBackend returns the global sub-backend mode ("wg" | "awg"), default wg. The
// profiles screen shows only profiles whose transport matches this.
func (s *Store) SubBackend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.SubBackend == "" {
		return "wg"
	}
	return s.data.SubBackend
}

// SetSubBackend persists the global sub-backend mode and keeps the active profile in
// sync: if the active profile is no longer of the selected transport, it switches to
// the first matching profile, or clears the active pointer when none exist.
func (s *Store) SetSubBackend(kind string) error {
	if kind != "awg" {
		kind = "wg"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.SubBackend = kind
	if !s.activeMatchesTransportLocked(kind) {
		s.data.ActiveID = s.firstOfTransportLocked(kind)
	}
	return s.saveLocked()
}

// Client returns the device-global client settings with defaults applied for any
// field left unset by an older store file.
func (s *Store) Client() ClientSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Client.withDefaults()
}

// SetClient persists the device-global client settings, normalizing scalars and the
// VK-links pool (trimmed, deduped, order-preserving). The pool is replaced wholesale
// here (the settings screen owns explicit edits/removals); import merges additively.
func (s *Store) SetClient(c ClientSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c = c.withDefaults()
	c.VKLinks = mergeVKLinks(nil, c.VKLinks)
	c.VKLinkSecondary = strings.TrimSpace(c.VKLinkSecondary)
	s.data.Client = c
	return s.saveLocked()
}

// AppRouting returns the per-app split-tunnel settings with the mode normalized.
func (s *Store) AppRouting() AppRoutingSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.AppRouting.withDefaults()
}

// SetAppRouting persists the per-app split-tunnel settings.
func (s *Store) SetAppRouting(a AppRoutingSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.AppRouting = a.withDefaults()
	return s.saveLocked()
}

func transportOf(p Profile) string {
	if p.TransportKind == "awg" {
		return "awg"
	}
	return "wg"
}

func (s *Store) activeMatchesTransportLocked(kind string) bool {
	for _, p := range s.data.Profiles {
		if p.ID == s.data.ActiveID {
			return transportOf(p) == kind
		}
	}
	return false
}

func (s *Store) firstOfTransportLocked(kind string) string {
	for _, p := range s.data.Profiles {
		if transportOf(p) == kind {
			return p.ID
		}
	}
	return ""
}

// Import decodes a wingsv:// link and adds every VK TURN profile it carries,
// deduping against existing servers. Returns the resulting stored profiles.
func (s *Store) Import(link string) ([]Profile, error) {
	// A raw awg-quick/wg-quick config (pasted straight from the clipboard) creates a
	// standalone transport profile; anything else is decoded as a wingsv:// link.
	if LooksLikeQuickConfig(link) {
		p := ProfileFromQuickConfig(link)
		id, err := s.Add(p)
		if err != nil {
			return nil, err
		}
		_ = id
		return s.List(), nil
	}
	cfg, err := wingsv.Decode(link)
	if err != nil {
		return nil, err
	}
	profiles := ProfilesFromConfig(cfg)
	if len(profiles) == 0 {
		return nil, errors.New("link carries no VK TURN profile")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstID string
	for i, p := range profiles {
		id := s.upsertLocked(p)
		if i == 0 {
			firstID = id
		}
		// The VK-links pool is device-global and shared across every profile: an
		// import only appends new links, never wipes the pool.
		s.data.Client.VKLinks = mergeVKLinks(s.data.Client.VKLinks, p.Links)
		if s.data.Client.VKLinkSecondary == "" && strings.TrimSpace(p.LinkSecondary) != "" {
			s.data.Client.VKLinkSecondary = strings.TrimSpace(p.LinkSecondary)
		}
	}
	// A fresh import activates the profile it just brought in (the first one for a
	// multi-profile subscription), so the just-added server is the one connected.
	if firstID != "" {
		s.data.ActiveID = firstID
	} else if s.data.ActiveID == "" && len(s.data.Profiles) > 0 {
		s.data.ActiveID = s.data.Profiles[0].ID
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return append([]Profile(nil), s.data.Profiles...), nil
}

// upsertLocked replaces a profile with the same server identity, preserving its id
// and favorite flag, or appends a new one with a fresh id.
func (s *Store) upsertLocked(p Profile) string {
	key := p.DedupKey()
	for i := range s.data.Profiles {
		if s.data.Profiles[i].DedupKey() == key {
			p.ID = s.data.Profiles[i].ID
			p.Favorite = s.data.Profiles[i].Favorite
			s.data.Profiles[i] = p
			return p.ID
		}
	}
	p.ID = newID()
	s.data.Profiles = append(s.data.Profiles, p)
	return p.ID
}

// Add appends a fresh profile with a new id and makes it active. Used for the
// "create profile manually" flow (no import), so no dedup is applied.
func (s *Store) Add(p Profile) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = newID()
	s.data.Profiles = append(s.data.Profiles, p)
	s.data.ActiveID = p.ID
	if err := s.saveLocked(); err != nil {
		return "", err
	}
	return p.ID, nil
}

// Update replaces the stored profile that shares the given profile's id, used when
// the settings screen edits the active profile's fields.
func (s *Store) Update(p Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Profiles {
		if s.data.Profiles[i].ID == p.ID {
			s.data.Profiles[i] = p
			return s.saveLocked()
		}
	}
	return errors.New("unknown profile")
}

// Activate marks the profile with the given id active.
func (s *Store) Activate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasLocked(id) {
		return errors.New("unknown profile")
	}
	s.data.ActiveID = id
	return s.saveLocked()
}

// ToggleFavorite flips the favorite flag of the profile with the given id.
func (s *Store) ToggleFavorite(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Profiles {
		if s.data.Profiles[i].ID == id {
			s.data.Profiles[i].Favorite = !s.data.Profiles[i].Favorite
			return s.saveLocked()
		}
	}
	return errors.New("unknown profile")
}

// Remove deletes the profile with the given id, clearing the active pointer if it
// pointed at the removed profile.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.Profiles[:0]
	found := false
	for _, p := range s.data.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return errors.New("unknown profile")
	}
	s.data.Profiles = kept
	if s.data.ActiveID == id {
		s.data.ActiveID = ""
	}
	return s.saveLocked()
}

func (s *Store) hasLocked(id string) bool {
	for _, p := range s.data.Profiles {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail; fall back to a fixed marker so the id is
		// still unique-per-append via the caller appending distinct profiles.
		return "id-" + hex.EncodeToString(b[:])
	}
	return hex.EncodeToString(b[:])
}
