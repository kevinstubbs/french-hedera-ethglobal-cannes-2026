package pipeline

import (
	"time"
)

// NaryoInboundEvent is a persisted payload from the Naryo → platform webhook.
type NaryoInboundEvent struct {
	EventID    string         `json:"eventId"`
	Payload    map[string]any `json:"payload,omitempty"`
	ReceivedAt time.Time      `json:"receivedAt"`
}

const maxNaryoEventsPerSession = 256

// AppendNaryoEvent stores an inbound event for a session. Duplicate eventId for the same session is ignored (idempotent).
func (s *MemoryStore) AppendNaryoEvent(sessionID, eventID string, payload map[string]any) (duplicate bool, err error) {
	if sessionID == "" || eventID == "" {
		return false, ErrInvalidNaryoEvent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return false, ErrNotFound
	}
	if s.naryoDedup == nil {
		s.naryoDedup = make(map[string]struct{})
	}
	if s.naryoEvents == nil {
		s.naryoEvents = make(map[string][]NaryoInboundEvent)
	}
	dk := sessionID + "\x00" + eventID
	if _, ok := s.naryoDedup[dk]; ok {
		return true, nil
	}
	s.naryoDedup[dk] = struct{}{}
	ev := NaryoInboundEvent{
		EventID:    eventID,
		Payload:    payload,
		ReceivedAt: time.Now().UTC(),
	}
	list := append(s.naryoEvents[sessionID], ev)
	if len(list) > maxNaryoEventsPerSession {
		list = list[len(list)-maxNaryoEventsPerSession:]
	}
	s.naryoEvents[sessionID] = list
	return false, nil
}

// NaryoEvents returns the most recent inbound events for a session (oldest first in the returned slice).
func (s *MemoryStore) NaryoEvents(sessionID string, limit int) ([]NaryoInboundEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > maxNaryoEventsPerSession {
		limit = maxNaryoEventsPerSession
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	list := s.naryoEvents[sessionID]
	if len(list) == 0 {
		return nil, nil
	}
	if len(list) <= limit {
		out := make([]NaryoInboundEvent, len(list))
		copy(out, list)
		return out, nil
	}
	out := make([]NaryoInboundEvent, limit)
	copy(out, list[len(list)-limit:])
	return out, nil
}
