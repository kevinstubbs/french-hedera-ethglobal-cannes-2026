package pipeline

import (
	"errors"
	"sync"
)

var ErrNotFound = errors.New("pipeline session not found")

// MemoryStore is a mutex-guarded map of sessions.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]*Session)}
}

// Put inserts or replaces a session pointer (caller-owned Session must not be mutated concurrently).
func (s *MemoryStore) Put(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

// Get returns a shallow copy so callers can mutate without racing other readers.
func (s *MemoryStore) Get(id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return *v, nil
}

// Update loads the session, calls fn, and saves if fn returns true.
func (s *MemoryStore) Update(id string, fn func(*Session) (changed bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return ErrNotFound
	}
	changed, err := fn(v)
	if err != nil {
		return err
	}
	if changed {
		s.sessions[id] = v
	}
	return nil
}

// IDs returns a snapshot of session keys for iteration (e.g. billing ticks).
func (s *MemoryStore) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.sessions))
	for k := range s.sessions {
		out = append(out, k)
	}
	return out
}

// List returns shallow copies of all sessions (arbitrary order).
func (s *MemoryStore) List() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Session, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, *v)
	}
	return out
}
