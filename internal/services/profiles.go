// Package services holds the Wails-bound services the Vue frontend calls. gRPC and
// filesystem work stays here on the Go side; the frontend only sees these methods.
package services

import (
	"errors"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/wingsv"
)

// ProfilesService exposes the VK TURN profile store to the frontend.
type ProfilesService struct {
	store    *config.Store
	onChange func() // notifies the connection service so it can live-patch or reconnect
}

// NewProfilesService wires a ProfilesService to a loaded store; onChange (may be nil)
// runs after every settings edit so a running tunnel can live-patch or reconnect.
func NewProfilesService(store *config.Store, onChange func()) *ProfilesService {
	return &ProfilesService{store: store, onChange: onChange}
}

func (s *ProfilesService) notify() {
	if s.onChange != nil {
		s.onChange()
	}
}

// ProfilesResult is the frontend-facing snapshot of the profile list.
type ProfilesResult struct {
	Profiles   []config.Profile      `json:"profiles"`
	ActiveID   string                `json:"activeId"`
	SubBackend string                `json:"subBackend"`
	Client     config.ClientSettings `json:"client"`
}

func (s *ProfilesService) snapshot() ProfilesResult {
	return ProfilesResult{
		Profiles:   s.store.List(),
		ActiveID:   s.store.ActiveID(),
		SubBackend: s.store.SubBackend(),
		Client:     s.store.Client(),
	}
}

// List returns the current profiles and the active id.
func (s *ProfilesService) List() ProfilesResult {
	return s.snapshot()
}

// ImportLink adds every VK TURN profile carried by a wingsv:// link.
func (s *ProfilesService) ImportLink(link string) (ProfilesResult, error) {
	if _, err := s.store.Import(link); err != nil {
		return ProfilesResult{}, err
	}
	return s.snapshot(), nil
}

// ExportActive returns the active profile encoded as a wingsv:// share link.
func (s *ProfilesService) ExportActive() (string, error) {
	activeID := s.store.ActiveID()
	if activeID != "" {
		for _, p := range s.store.List() {
			if p.ID == activeID {
				return wingsv.Encode(p.ToConfig())
			}
		}
	}
	return "", errors.New("no active profile")
}

// Update saves edits to a profile (used by the VK TURN settings screen).
func (s *ProfilesService) Update(profile config.Profile) (ProfilesResult, error) {
	if err := s.store.Update(profile); err != nil {
		return ProfilesResult{}, err
	}
	s.notify()
	return s.snapshot(), nil
}

// SetSubBackend switches the global sub-backend mode ("wg" | "awg"). Profiles are
// intrinsically typed by their import; this only changes which profiles the screen
// shows and which transport is used to connect.
func (s *ProfilesService) SetSubBackend(kind string) (ProfilesResult, error) {
	if err := s.store.SetSubBackend(kind); err != nil {
		return ProfilesResult{}, err
	}
	s.notify()
	return s.snapshot(), nil
}

// CreateProfile adds a blank profile of the current sub-backend transport and makes
// it active, so a config can be filled in by hand instead of imported.
func (s *ProfilesService) CreateProfile() (ProfilesResult, error) {
	kind := s.store.SubBackend()
	title := "Пустой профиль (WireGuard)"
	if kind == "awg" {
		title = "Пустой профиль (AmneziaWG)"
	}
	p := config.Profile{
		Title:         title,
		TransportKind: kind,
		Settings:      config.DefaultSettings(),
		WG:            config.DefaultWireGuard(),
	}
	if _, err := s.store.Add(p); err != nil {
		return ProfilesResult{}, err
	}
	return s.snapshot(), nil
}

// SetClientSettings persists the device-global client parameters (VK-links pool,
// threads, creds group size, VK auth mode, session mode, browser fingerprint).
func (s *ProfilesService) SetClientSettings(client config.ClientSettings) (ProfilesResult, error) {
	if err := s.store.SetClient(client); err != nil {
		return ProfilesResult{}, err
	}
	s.notify()
	return s.snapshot(), nil
}

// Activate marks a profile active by id.
func (s *ProfilesService) Activate(id string) (ProfilesResult, error) {
	if err := s.store.Activate(id); err != nil {
		return ProfilesResult{}, err
	}
	s.notify()
	return s.snapshot(), nil
}

// ToggleFavorite flips a profile's favorite flag.
func (s *ProfilesService) ToggleFavorite(id string) (ProfilesResult, error) {
	if err := s.store.ToggleFavorite(id); err != nil {
		return ProfilesResult{}, err
	}
	return s.snapshot(), nil
}

// Remove deletes a profile by id.
func (s *ProfilesService) Remove(id string) (ProfilesResult, error) {
	if err := s.store.Remove(id); err != nil {
		return ProfilesResult{}, err
	}
	return s.snapshot(), nil
}
