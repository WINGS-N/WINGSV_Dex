package services

import "github.com/WINGS-N/wingsv-dex/internal/applog"

// LogLineEvent carries one appended log line to the frontend for live display (no re-fetch).
const LogLineEvent = "logs:line"

// LogLine is the payload for LogLineEvent.
type LogLine struct {
	Channel string `json:"channel"`
	Line    string `json:"line"`
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
