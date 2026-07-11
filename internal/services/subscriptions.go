package services

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/xraysubs"
)

// SubscriptionsUpdatedEvent carries the fresh subscription list to the frontend after a
// background auto-update, so an open Subscriptions screen reflects it without polling.
const SubscriptionsUpdatedEvent = "subscriptions:updated"

// defaultRefreshMinutes is used when a subscription sets no explicit interval.
const defaultRefreshMinutes = 720

// SubscriptionService manages xray subscriptions: fetch, parse, and scheduled auto-update.
type SubscriptionService struct {
	store *config.Store
	app   *application.App
}

// NewSubscriptionService wires a SubscriptionService to the store.
func NewSubscriptionService(store *config.Store) *SubscriptionService {
	return &SubscriptionService{store: store}
}

// SetApp attaches the app so background refreshes can push events to the frontend.
func (s *SubscriptionService) SetApp(app *application.App) { s.app = app }

// SubscriptionRefresh is returned after a refresh so the UI can show what changed.
type SubscriptionRefresh struct {
	Subscriptions []config.Subscription `json:"subscriptions"`
	Updated       int                   `json:"updated"`
	Error         string                `json:"error,omitempty"`
}

// List returns the stored subscriptions.
func (s *SubscriptionService) List() []config.Subscription {
	return s.store.SubscriptionList()
}

// Add adds a subscription and immediately fetches it.
func (s *SubscriptionService) Add(title, url string) (SubscriptionRefresh, error) {
	sub, err := s.store.AddSubscription(title, url)
	if err != nil {
		return SubscriptionRefresh{}, err
	}
	n, ferr := s.refresh(sub)
	res := SubscriptionRefresh{Subscriptions: s.store.SubscriptionList(), Updated: n}
	if ferr != nil {
		res.Error = ferr.Error()
	}
	return res, nil
}

// Remove deletes a subscription and its nodes.
func (s *SubscriptionService) Remove(id string) ([]config.Subscription, error) {
	if err := s.store.RemoveSubscription(id); err != nil {
		return nil, err
	}
	return s.store.SubscriptionList(), nil
}

// SetAutoUpdate toggles a subscription's auto-update flag.
func (s *SubscriptionService) SetAutoUpdate(id string, on bool) ([]config.Subscription, error) {
	if err := s.store.SetSubscriptionAutoUpdate(id, on); err != nil {
		return nil, err
	}
	return s.store.SubscriptionList(), nil
}

// Refresh fetches one subscription by id.
func (s *SubscriptionService) Refresh(id string) (SubscriptionRefresh, error) {
	for _, sub := range s.store.SubscriptionList() {
		if sub.ID == id {
			n, err := s.refresh(sub)
			res := SubscriptionRefresh{Subscriptions: s.store.SubscriptionList(), Updated: n}
			if err != nil {
				res.Error = err.Error()
			}
			return res, nil
		}
	}
	return SubscriptionRefresh{Subscriptions: s.store.SubscriptionList()}, nil
}

// RefreshAll fetches every subscription.
func (s *SubscriptionService) RefreshAll() SubscriptionRefresh {
	total := 0
	var lastErr string
	for _, sub := range s.store.SubscriptionList() {
		if n, err := s.refresh(sub); err != nil {
			lastErr = err.Error()
		} else {
			total += n
		}
	}
	return SubscriptionRefresh{Subscriptions: s.store.SubscriptionList(), Updated: total, Error: lastErr}
}

// refresh fetches a single subscription and stores its nodes + quota.
func (s *SubscriptionService) refresh(sub config.Subscription) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	res, err := xraysubs.Fetch(ctx, sub)
	if err != nil {
		return 0, err
	}
	q := res.Quota
	if err := s.store.ApplySubscriptionNodes(sub.ID, res.Nodes, q.Upload, q.Download, q.Total, q.Expire); err != nil {
		return 0, err
	}
	return len(res.Nodes), nil
}

// StartAutoUpdate launches the background scheduler: an initial due-refresh shortly after
// startup, then a periodic sweep that refreshes auto-update subscriptions whose interval
// has elapsed. It emits SubscriptionsUpdatedEvent whenever it refreshes anything.
func (s *SubscriptionService) StartAutoUpdate() {
	go func() {
		time.Sleep(20 * time.Second)
		s.sweep()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.sweep()
		}
	}()
}

func (s *SubscriptionService) sweep() {
	now := time.Now().Unix()
	refreshed := false
	for _, sub := range s.store.SubscriptionList() {
		if !sub.AutoUpdate || !due(sub, now) {
			continue
		}
		if _, err := s.refresh(sub); err == nil {
			refreshed = true
		}
	}
	if refreshed && s.app != nil {
		s.app.Event.Emit(SubscriptionsUpdatedEvent, s.store.SubscriptionList())
	}
}

// due reports whether a subscription's refresh interval has elapsed.
func due(sub config.Subscription, now int64) bool {
	if sub.LastUpdatedAt == 0 {
		return true
	}
	minutes := sub.RefreshIntervalMinutes
	if minutes <= 0 {
		minutes = defaultRefreshMinutes
	}
	return now-sub.LastUpdatedAt >= int64(minutes)*60
}
