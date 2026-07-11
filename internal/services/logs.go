package services

import "github.com/WINGS-N/wingsv-dex/internal/applog"

const LogsUpdatedEvent = "logs:updated"

// LogUpdate tells the frontend which log channel changed.
type LogUpdate struct {
	Channel string `json:"channel"`
}

// LogsService exposes bounded runtime/proxy logs to the frontend.
type LogsService struct {
	store *applog.Store
}

func NewLogsService(store *applog.Store) *LogsService {
	return &LogsService{store: store}
}

func (s *LogsService) Snapshot(channel string) (applog.Snapshot, error) {
	return s.store.Snapshot(channel)
}

func (s *LogsService) Clear(channel string) error {
	return s.store.Clear(channel)
}
