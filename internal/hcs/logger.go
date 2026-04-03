package hcs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Event is a pipeline or billing record for HCS (local Hedera in later phases).
type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	SessionID string         `json:"sessionId,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// Logger emits structured HCS-oriented events (stdout stub until Hedera writer exists).
type Logger struct {
	log *slog.Logger
}

// NewLogger returns a JSON-logging HCS stub.
func NewLogger() *Logger {
	return &Logger{log: slog.Default()}
}

// Emit writes one JSON line with the event payload.
func (l *Logger) Emit(ctx context.Context, e Event) {
	if l == nil {
		return
	}
	e.Timestamp = time.Now().UTC()
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	b, err := json.Marshal(e)
	if err != nil {
		slog.Error("hcs: marshal event", "err", err)
		return
	}
	l.log.InfoContext(ctx, string(b))
}
