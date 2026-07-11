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
	"time"

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
	Profiles       []Profile          `json:"profiles"`
	ActiveID       string             `json:"activeId"`
	SubBackend     string             `json:"subBackend"`     // "wg" | "awg" global mode
	NetworkBackend string             `json:"networkBackend"` // "vk_turn" | "xray"
	Client         ClientSettings     `json:"client"`         // device-global VK TURN params + VK-links pool
	AppRouting     AppRoutingSettings `json:"appRouting"`     // per-app split-tunnel mode + lists

	// Xray backend state, parallel to the VK TURN profiles above.
	XrayProfiles     []XrayProfile  `json:"xrayProfiles"`
	XrayActiveID     string         `json:"xrayActiveId"`
	XraySettings     XraySettings   `json:"xraySettings"`
	Subscriptions    []Subscription `json:"subscriptions"`
	DefaultSubSeeded bool           `json:"defaultSubSeeded"` // the built-in Universal sub was added once
}

// NewStore loads the store from path, creating an empty one if the file is absent.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Fresh store: seed the device-global defaults so bool-default fields (e.g.
		// restart-on-network-change) start true rather than at their zero value.
		s.data.Client = DefaultClientSettings()
		s.data.NetworkBackend = BackendVKTurn
		s.data.XraySettings = DefaultXraySettings()
		s.data.XraySettings.ensureCreds()
		s.seedDefaultSubscriptionLocked()
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
	// Seed the built-in Universal subscription once, on this and older stores alike.
	if !s.data.DefaultSubSeeded {
		s.seedDefaultSubscriptionLocked()
		_ = s.saveLocked()
	}
	return s, nil
}

// seedDefaultSubscriptionLocked adds the built-in Universal subscription if it is not
// already present and marks it seeded so a user who deletes it is not re-seeded.
func (s *Store) seedDefaultSubscriptionLocked() {
	s.data.DefaultSubSeeded = true
	for _, sub := range s.data.Subscriptions {
		if strings.TrimSpace(sub.URL) == DefaultSubscriptionURL {
			return
		}
	}
	s.data.Subscriptions = append(s.data.Subscriptions, Subscription{
		ID:         newID(),
		Title:      DefaultSubscriptionTitle,
		URL:        DefaultSubscriptionURL,
		AutoUpdate: true,
	})
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

// NetworkBackend returns the active network backend ("vk_turn" | "xray"), default vk_turn.
func (s *Store) NetworkBackend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.NetworkBackend == BackendXray {
		return BackendXray
	}
	return BackendVKTurn
}

// SetNetworkBackend persists the active network backend.
func (s *Store) SetNetworkBackend(kind string) error {
	if kind != BackendXray {
		kind = BackendVKTurn
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.NetworkBackend = kind
	return s.saveLocked()
}

// XrayList returns a copy of the stored Xray profiles.
func (s *Store) XrayList() []XrayProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]XrayProfile(nil), s.data.XrayProfiles...)
}

// XrayActiveID returns the id of the active Xray profile, or empty if none.
func (s *Store) XrayActiveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.XrayActiveID
}

// XrayActivate marks the Xray profile with the given id active.
func (s *Store) XrayActivate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.XrayProfiles {
		if p.ID == id {
			s.data.XrayActiveID = id
			return s.saveLocked()
		}
	}
	return errors.New("unknown profile")
}

// XrayToggleFavorite flips the favorite flag of the Xray profile with the given id.
func (s *Store) XrayToggleFavorite(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.XrayProfiles {
		if s.data.XrayProfiles[i].ID == id {
			s.data.XrayProfiles[i].Favorite = !s.data.XrayProfiles[i].Favorite
			return s.saveLocked()
		}
	}
	return errors.New("unknown profile")
}

// XrayRemove deletes the Xray profile with the given id, clearing the active pointer if
// it pointed at the removed profile.
func (s *Store) XrayRemove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.XrayProfiles[:0]
	found := false
	for _, p := range s.data.XrayProfiles {
		if p.ID == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return errors.New("unknown profile")
	}
	s.data.XrayProfiles = kept
	if s.data.XrayActiveID == id {
		s.data.XrayActiveID = ""
	}
	return s.saveLocked()
}

// XraySettings returns the Xray settings, backstopping scalar defaults and lazily
// generating the local SOCKS/HTTP credentials (persisting them on first read).
func (s *Store) XraySettings() XraySettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.data.XraySettings.withDefaults()
	if x.ensureCreds() {
		s.data.XraySettings = x
		_ = s.saveLocked()
	}
	return x
}

// SetXraySettings persists the Xray settings, backstopping defaults and ensuring the
// local proxy credentials exist when auth is enabled.
func (s *Store) SetXraySettings(x XraySettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	x = x.withDefaults()
	x.ensureCreds()
	s.data.XraySettings = x
	return s.saveLocked()
}

// SubscriptionList returns a copy of the stored subscriptions.
func (s *Store) SubscriptionList() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Subscription(nil), s.data.Subscriptions...)
}

// AddSubscription adds (or updates by URL) a subscription and returns it.
func (s *Store) AddSubscription(title, url string) (Subscription, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Subscription{}, errors.New("empty subscription url")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := Subscription{Title: strings.TrimSpace(title), URL: url, AutoUpdate: true}
	id := s.upsertSubscriptionLocked(sub)
	if err := s.saveLocked(); err != nil {
		return Subscription{}, err
	}
	for _, x := range s.data.Subscriptions {
		if x.ID == id {
			return x, nil
		}
	}
	return sub, nil
}

// RemoveSubscription deletes a subscription and every Xray profile that came from it.
func (s *Store) RemoveSubscription(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keptSubs := s.data.Subscriptions[:0]
	found := false
	for _, sub := range s.data.Subscriptions {
		if sub.ID == id {
			found = true
			continue
		}
		keptSubs = append(keptSubs, sub)
	}
	if !found {
		return errors.New("unknown subscription")
	}
	s.data.Subscriptions = keptSubs
	s.dropSubscriptionProfilesLocked(id)
	return s.saveLocked()
}

// SetSubscriptionAutoUpdate toggles a subscription's auto-update flag.
func (s *Store) SetSubscriptionAutoUpdate(id string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Subscriptions {
		if s.data.Subscriptions[i].ID == id {
			s.data.Subscriptions[i].AutoUpdate = on
			return s.saveLocked()
		}
	}
	return errors.New("unknown subscription")
}

// ApplySubscriptionNodes replaces the Xray profiles belonging to a subscription with a
// freshly fetched set, updates its last-updated time and advertised quota, and preserves
// the favorite flag / active pointer for nodes whose raw link is unchanged. An empty
// nodes slice prunes the subscription's profiles (server returned none).
func (s *Store) ApplySubscriptionNodes(id string, nodes []XrayProfile, upload, download, total, expire int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sub *Subscription
	for i := range s.data.Subscriptions {
		if s.data.Subscriptions[i].ID == id {
			sub = &s.data.Subscriptions[i]
			break
		}
	}
	if sub == nil {
		return errors.New("unknown subscription")
	}

	// Carry over favorites keyed by raw link before dropping the old set.
	fav := map[string]bool{}
	for _, p := range s.data.XrayProfiles {
		if p.SubscriptionID == id && p.Favorite {
			fav[p.DedupKey()] = true
		}
	}
	activeLink := ""
	for _, p := range s.data.XrayProfiles {
		if p.ID == s.data.XrayActiveID {
			activeLink = p.DedupKey()
		}
	}

	s.dropSubscriptionProfilesLocked(id)
	for _, n := range nodes {
		n.SubscriptionID = id
		n.SubscriptionTitle = sub.Title
		n.Favorite = fav[n.DedupKey()]
		newID := s.upsertXrayLocked(n)
		if activeLink != "" && n.DedupKey() == activeLink {
			s.data.XrayActiveID = newID
		}
	}

	sub.LastUpdatedAt = time.Now().Unix()
	sub.AdvertisedUploadBytes = upload
	sub.AdvertisedDownloadBytes = download
	sub.AdvertisedTotalBytes = total
	sub.AdvertisedExpireAt = expire
	return s.saveLocked()
}

func (s *Store) dropSubscriptionProfilesLocked(id string) {
	kept := s.data.XrayProfiles[:0]
	for _, p := range s.data.XrayProfiles {
		if p.SubscriptionID == id {
			if p.ID == s.data.XrayActiveID {
				s.data.XrayActiveID = ""
			}
			continue
		}
		kept = append(kept, p)
	}
	s.data.XrayProfiles = kept
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

// ImportXray adds Xray nodes from either a raw xray share link (vless://..., possibly
// several whitespace-separated) or an Xray-backed wingsv:// link (which also carries
// settings and subscriptions). Switches the active network backend to Xray.
func (s *Store) ImportXray(link string) ([]XrayProfile, error) {
	link = strings.TrimSpace(link)
	var profiles []XrayProfile
	var settings *XraySettings
	var subs []Subscription

	if LooksLikeXrayLink(link) {
		for _, ln := range strings.Fields(link) {
			if p, ok := ParseShareLink(ln); ok {
				profiles = append(profiles, p)
			}
		}
	} else {
		cfg, err := wingsv.Decode(link)
		if err != nil {
			return nil, err
		}
		profiles = XrayProfilesFromConfig(cfg)
		subs = SubscriptionsFromConfig(cfg)
		if xs := cfg.GetXray().GetSettings(); xs != nil {
			v := XraySettingsFromProto(xs)
			settings = &v
		}
	}
	if len(profiles) == 0 {
		return nil, errors.New("link carries no Xray profile")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var firstID string
	for i, p := range profiles {
		id := s.upsertXrayLocked(p)
		if i == 0 {
			firstID = id
		}
	}
	for _, sub := range subs {
		s.upsertSubscriptionLocked(sub)
	}
	if settings != nil {
		settings.ensureCreds()
		s.data.XraySettings = settings.withDefaults()
	}
	if firstID != "" {
		s.data.XrayActiveID = firstID
	}
	s.data.NetworkBackend = BackendXray
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return append([]XrayProfile(nil), s.data.XrayProfiles...), nil
}

// upsertXrayLocked replaces an Xray profile sharing the same raw link (preserving id and
// favorite), or appends a new one with a fresh id.
func (s *Store) upsertXrayLocked(p XrayProfile) string {
	key := p.DedupKey()
	for i := range s.data.XrayProfiles {
		if s.data.XrayProfiles[i].DedupKey() == key {
			p.ID = s.data.XrayProfiles[i].ID
			p.Favorite = s.data.XrayProfiles[i].Favorite
			s.data.XrayProfiles[i] = p
			return p.ID
		}
	}
	p.ID = newID()
	s.data.XrayProfiles = append(s.data.XrayProfiles, p)
	return p.ID
}

// upsertSubscriptionLocked replaces a subscription with the same URL (preserving id), or
// appends a new one.
func (s *Store) upsertSubscriptionLocked(sub Subscription) string {
	url := strings.TrimSpace(sub.URL)
	for i := range s.data.Subscriptions {
		if strings.TrimSpace(s.data.Subscriptions[i].URL) == url && url != "" {
			sub.ID = s.data.Subscriptions[i].ID
			s.data.Subscriptions[i] = sub
			return sub.ID
		}
	}
	if sub.ID == "" {
		sub.ID = newID()
	}
	s.data.Subscriptions = append(s.data.Subscriptions, sub)
	return sub.ID
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
