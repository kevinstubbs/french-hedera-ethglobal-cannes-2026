package pipeline

import (
	"sync"
	"time"
)

// ActivityEntry is one row in the in-memory observability feed (lifecycle + payment stream).
type ActivityEntry struct {
	Time      time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	SessionID string         `json:"sessionId,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// ActivityLog is a fixed-capacity ring of recent events for dashboards.
type ActivityLog struct {
	mu      sync.Mutex
	max     int
	entries []ActivityEntry
}

// NewActivityLog returns a ring buffer holding up to max entries (minimum 16).
func NewActivityLog(max int) *ActivityLog {
	if max < 16 {
		max = 16
	}
	return &ActivityLog{max: max, entries: make([]ActivityEntry, 0, max)}
}

// Record appends an event (drops oldest when full).
func (a *ActivityLog) Record(e ActivityEntry) {
	if a == nil {
		return
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.entries) >= a.max {
		copy(a.entries, a.entries[1:])
		a.entries = a.entries[:len(a.entries)-1]
	}
	a.entries = append(a.entries, e)
}

// Snapshot returns newest-first copy of recent entries (limit capped to max).
func (a *ActivityLog) Snapshot(limit int) []ActivityEntry {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 || limit > len(a.entries) {
		limit = len(a.entries)
	}
	start := len(a.entries) - limit
	out := make([]ActivityEntry, limit)
	for i := 0; i < limit; i++ {
		out[limit-1-i] = a.entries[start+i]
	}
	return out
}
