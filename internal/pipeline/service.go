package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/hcs"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
)

var (
	ErrInvalidTransition = errors.New("invalid pipeline state transition")
)

// Service coordinates pipeline sessions, Naryo, and billing notifications.
type Service struct {
	store    *MemoryStore
	naryo    naryo.Client
	hcs      *hcs.Logger
	activity *ActivityLog
	rate     int64 // cents per second for billing metadata
}

// NewService wires dependencies. rateCentsPerSecond defaults to 1 if <= 0.
// activity may be nil; when set, lifecycle events are mirrored for dashboards.
func NewService(store *MemoryStore, client naryo.Client, log *hcs.Logger, rateCentsPerSecond int64, activity *ActivityLog) *Service {
	if rateCentsPerSecond <= 0 {
		rateCentsPerSecond = 1
	}
	return &Service{
		store:    store,
		naryo:    client,
		hcs:      log,
		activity: activity,
		rate:     rateCentsPerSecond,
	}
}

func newSessionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Create registers a new session in created state.
func (s *Service) Create(ctx context.Context, agentID string) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:                  id,
		AgentID:             agentID,
		State:               StateCreated,
		PaymentStreamActive: false,
		RateCentsPerSecond:  s.rate,
	}
	s.store.Put(sess)
	s.emit(ctx, "pipeline_created", id, map[string]any{"agentId": agentID})
	return sess, nil
}

// Start moves created or paused -> running and provisions Naryo.
func (s *Service) Start(ctx context.Context, id string) error {
	var opID string
	err := s.store.Update(id, func(v *Session) (bool, error) {
		if v.State != StateCreated && v.State != StatePaused {
			return false, fmt.Errorf("%w: start from %s", ErrInvalidTransition, v.State)
		}
		op, err := s.naryo.EnsurePipeline(ctx, id)
		if err != nil {
			return false, err
		}
		opID = op
		v.State = StateRunning
		v.PaymentStreamActive = true
		v.LastNaryoOpID = op
		return true, nil
	})
	if err != nil {
		return err
	}
	s.emit(ctx, "pipeline_started", id, map[string]any{"naryoOpId": opID})
	return nil
}

// Stop ends the session.
func (s *Service) Stop(ctx context.Context, id string) error {
	if _, err := s.store.Get(id); err != nil {
		return err
	}
	err := s.naryo.StopPipeline(ctx, id)
	if err != nil {
		return err
	}
	err = s.store.Update(id, func(v *Session) (bool, error) {
		if v.State == StateStopped {
			return false, nil
		}
		v.State = StateStopped
		v.PaymentStreamActive = false
		return true, nil
	})
	if err != nil {
		return err
	}
	s.emit(ctx, "pipeline_stopped", id, nil)
	return nil
}

// Pause pauses egress and stops billing ticks (stream treated as inactive).
func (s *Service) Pause(ctx context.Context, id string) error {
	err := s.naryo.PauseEgress(ctx, id)
	if err != nil {
		return err
	}
	err = s.store.Update(id, func(v *Session) (bool, error) {
		if v.State != StateRunning {
			return false, fmt.Errorf("%w: pause from %s", ErrInvalidTransition, v.State)
		}
		v.State = StatePaused
		v.PaymentStreamActive = false
		return true, nil
	})
	if err != nil {
		return err
	}
	s.emit(ctx, "pipeline_paused", id, nil)
	return nil
}

// Resume moves paused -> running and re-enables billing when stream is active.
func (s *Service) Resume(ctx context.Context, id string) error {
	err := s.naryo.ResumeEgress(ctx, id)
	if err != nil {
		return err
	}
	err = s.store.Update(id, func(v *Session) (bool, error) {
		if v.State != StatePaused {
			return false, fmt.Errorf("%w: resume from %s", ErrInvalidTransition, v.State)
		}
		v.State = StateRunning
		v.PaymentStreamActive = true
		return true, nil
	})
	if err != nil {
		return err
	}
	s.emit(ctx, "pipeline_resumed", id, nil)
	return nil
}

// Reconfigure is a placeholder for intent/template updates (Phase 1 no-op beyond HCS).
func (s *Service) Reconfigure(ctx context.Context, id string, _ map[string]any) error {
	_, err := s.store.Get(id)
	if err != nil {
		return err
	}
	s.emit(ctx, "pipeline_reconfigured", id, map[string]any{"phase": 1})
	return nil
}

// SetPaymentStreamActive updates stream flag (simulates x402 payment stream for billing).
func (s *Service) SetPaymentStreamActive(ctx context.Context, id string, active bool) error {
	err := s.store.Update(id, func(v *Session) (bool, error) {
		if v.State != StateRunning {
			return false, fmt.Errorf("%w: stream toggle in state %s", ErrInvalidTransition, v.State)
		}
		if v.PaymentStreamActive == active {
			return false, nil
		}
		v.PaymentStreamActive = active
		return true, nil
	})
	if err != nil {
		return err
	}
	s.emit(ctx, "payment_stream", id, map[string]any{"active": active})
	return nil
}

// Status returns a copy of the session.
func (s *Service) Status(id string) (Session, error) {
	return s.store.Get(id)
}

// ListSessions returns all pipeline sessions (for observability).
func (s *Service) ListSessions() []Session {
	return s.store.List()
}

// ActivityFeed returns recent observability events, newest first.
func (s *Service) ActivityFeed(limit int) []ActivityEntry {
	if s.activity == nil {
		return nil
	}
	return s.activity.Snapshot(limit)
}

// RunBillingLoop runs billing ticks every interval until ctx is cancelled.
func (s *Service) RunBillingLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runBillingTick(ctx)
		}
	}
}

// BillingTick runs a single billing pass (for tests and custom schedulers).
func (s *Service) BillingTick(ctx context.Context) {
	s.runBillingTick(ctx)
}

// runBillingTick increments billed seconds for running sessions with an active payment stream.
func (s *Service) runBillingTick(ctx context.Context) {
	for _, id := range s.store.IDs() {
		_ = s.store.Update(id, func(v *Session) (bool, error) {
			if v.State != StateRunning || !v.PaymentStreamActive {
				return false, nil
			}
			v.BilledSeconds++
			if s.hcs != nil {
				s.hcs.Emit(ctx, hcs.Event{
					Type:      "billing_tick",
					SessionID: id,
					Data: map[string]any{
						"billedSeconds":      v.BilledSeconds,
						"rateCentsPerSecond": v.RateCentsPerSecond,
					},
				})
			}
			return true, nil
		})
	}
}

func (s *Service) emit(ctx context.Context, typ, sessionID string, data map[string]any) {
	if s.activity != nil {
		var cp map[string]any
		if len(data) > 0 {
			cp = make(map[string]any, len(data))
			for k, v := range data {
				cp[k] = v
			}
		}
		s.activity.Record(ActivityEntry{Type: typ, SessionID: sessionID, Data: cp})
	}
	if s.hcs == nil {
		return
	}
	s.hcs.Emit(ctx, hcs.Event{Type: typ, SessionID: sessionID, Data: data})
}
