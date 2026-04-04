package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/ledger"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
)

var (
	ErrInvalidTransition = errors.New("invalid pipeline state transition")
)

// HCSLogger is the subset of HCS logging used by the pipeline service (avoid import cycles in tests).
type HCSLogger interface {
	PipelineCreated(ctx context.Context, sessionID, agentID string)
	PipelineStarted(ctx context.Context, sessionID, agentID, naryoOpID string)
	PipelinePaused(ctx context.Context, sessionID, reason string, remainingBalanceUnits int64)
	PipelineResumed(ctx context.Context, sessionID string)
	PipelineStopped(ctx context.Context, sessionID string)
	PipelineReconfigured(ctx context.Context, sessionID string, phase int)
	BillingTick(ctx context.Context, sessionID string, billedSeconds, rateCentsPerSecond int64)
	BillingSummary(ctx context.Context, sessionID string, args BillingSummaryArgs)
	AgentTopUp(ctx context.Context, args AgentTopUpArgs)
	StartRejectedInsufficientPrepaid(ctx context.Context, sessionID string, agentID string, requiredUnits, remainingUnits int64)
	PaymentStreamStarted(ctx context.Context, sessionID string)
	PaymentStreamStalled(ctx context.Context, sessionID string)
	PaymentStreamTerminated(ctx context.Context, sessionID string)
}

// Service coordinates pipeline sessions, Naryo, and billing notifications.
type Service struct {
	store    *MemoryStore
	naryo    naryo.Client
	hcs      HCSLogger
	activity *ActivityLog
	rate     int64 // cents per second for billing metadata

	ledger               PrepaidLedger
	rateUnitsPerMinute   int64
	summaryWindowMinutes int64
	now                  func() time.Time
}

// ServiceOption configures [NewService].
type ServiceOption func(*Service)

// WithPrepaidLedger sets the off-chain prepaid ledger (nil skips balance enforcement).
func WithPrepaidLedger(l PrepaidLedger) ServiceOption {
	return func(s *Service) {
		s.ledger = l
	}
}

// WithRateUnitsPerMinute sets units charged per wall-clock minute while running (default 60).
func WithRateUnitsPerMinute(n int64) ServiceOption {
	return func(s *Service) {
		s.rateUnitsPerMinute = n
	}
}

// WithSummaryWindowMinutes sets the HCS billing_summary batch window (clamped 5–15, default 10).
func WithSummaryWindowMinutes(n int64) ServiceOption {
	return func(s *Service) {
		s.summaryWindowMinutes = n
	}
}

// WithClock sets wall clock for billing minute boundaries (tests).
func WithClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		s.now = now
	}
}

// NewService wires dependencies. rateCentsPerSecond defaults to 1 if <= 0.
// activity may be nil; when set, lifecycle events are mirrored for dashboards.
func NewService(store *MemoryStore, client naryo.Client, log HCSLogger, rateCentsPerSecond int64, activity *ActivityLog, opts ...ServiceOption) *Service {
	if rateCentsPerSecond <= 0 {
		rateCentsPerSecond = 1
	}
	s := &Service{
		store:                store,
		naryo:                client,
		hcs:                  log,
		activity:             activity,
		rate:                 rateCentsPerSecond,
		rateUnitsPerMinute:   60,
		summaryWindowMinutes: 10,
		now:                  time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	s.summaryWindowMinutes = clampSummaryWin(s.summaryWindowMinutes)
	if s.rateUnitsPerMinute <= 0 {
		s.rateUnitsPerMinute = 60
	}
	return s
}

func clampSummaryWin(n int64) int64 {
	switch {
	case n < 5:
		return 5
	case n > 15:
		return 15
	default:
		return n
	}
}

func (s *Service) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func newSessionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PrepaidBalance returns the current prepaid balance for an agent (0 if no ledger).
func (s *Service) PrepaidBalance(agentID string) int64 {
	if s == nil || s.ledger == nil || agentID == "" {
		return 0
	}
	b, err := s.ledger.GetBalance(agentID)
	if err != nil {
		return 0
	}
	return b
}

// CreditTopUp credits prepaid balance and emits HCS (best-effort).
func (s *Service) CreditTopUp(ctx context.Context, args AgentTopUpArgs) error {
	if s == nil || s.ledger == nil {
		return errors.New("prepaid ledger not configured")
	}
	if args.AgentID == "" || args.AmountUnits <= 0 {
		return errors.New("invalid top-up")
	}
	if err := s.ledger.Credit(args.AgentID, args.AmountUnits, args.SourceTxID, args.Source); err != nil {
		return err
	}
	if s.hcs != nil {
		s.hcs.AgentTopUp(ctx, args)
	}
	s.recordActivity("agent_top_up", "", map[string]any{
		"agentId": args.AgentID, "amountUnits": args.AmountUnits, "source": args.Source,
	})
	return nil
}

func (s *Service) checkPrepaid(agentID string) error {
	if s.ledger == nil || agentID == "" {
		return nil
	}
	min := s.rateUnitsPerMinute
	if min <= 0 {
		min = 60
	}
	ok, bal, err := s.ledger.CanAfford(agentID, min)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: have %d need %d", ErrInsufficientPrepaid, bal, min)
	}
	return nil
}

// Create registers a new session in created state.
func (s *Service) Create(ctx context.Context, agentID string) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	rum := s.rateUnitsPerMinute
	if rum <= 0 {
		rum = 60
	}
	swin := s.summaryWindowMinutes
	if swin <= 0 {
		swin = 10
	}
	sess := &Session{
		ID:                   id,
		AgentID:              agentID,
		State:                StateCreated,
		PaymentStreamActive:  false,
		RateCentsPerSecond:   s.rate,
		RateUnitsPerMinute:   rum,
		SummaryWindowMinutes: swin,
	}
	s.store.Put(sess)
	s.recordActivity("pipeline_created", id, map[string]any{"agentId": agentID})
	if s.hcs != nil {
		s.hcs.PipelineCreated(ctx, id, agentID)
	}
	return sess, nil
}

// Start moves created or paused -> running and provisions Naryo.
func (s *Service) Start(ctx context.Context, id string) error {
	sess, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if err := s.checkPrepaid(sess.AgentID); err != nil {
		if errors.Is(err, ErrInsufficientPrepaid) && s.hcs != nil {
			var rem int64
			if s.ledger != nil {
				rem, _ = s.ledger.GetBalance(sess.AgentID)
			}
			min := s.rateUnitsPerMinute
			if min <= 0 {
				min = 60
			}
			s.hcs.StartRejectedInsufficientPrepaid(ctx, id, sess.AgentID, min, rem)
		}
		return err
	}

	var opID string
	var agentID string
	err = s.store.Update(id, func(v *Session) (bool, error) {
		agentID = v.AgentID
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
		v.LastPaidMinute = 0
		v.FundsChargeRetryPending = false
		v.FundsRetryAtMinute = 0
		v.AutoPausedForFunds = false
		return true, nil
	})
	if err != nil {
		return err
	}
	s.recordActivity("pipeline_started", id, map[string]any{"naryoOpId": opID, "agentId": agentID})
	if s.hcs != nil {
		s.hcs.PipelineStarted(ctx, id, agentID, opID)
	}
	return nil
}

// Stop ends the session.
func (s *Service) Stop(ctx context.Context, id string) error {
	sess, err := s.store.Get(id)
	if err != nil {
		return err
	}
	streamWasActive := sess.PaymentStreamActive
	err = s.naryo.StopPipeline(ctx, id)
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
	if streamWasActive && s.hcs != nil {
		s.hcs.PaymentStreamTerminated(ctx, id)
	}
	s.recordActivity("pipeline_stopped", id, nil)
	if s.hcs != nil {
		s.hcs.PipelineStopped(ctx, id)
	}
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
	s.recordActivity("pipeline_paused", id, nil)
	if s.hcs != nil {
		s.hcs.PipelinePaused(ctx, id, "", 0)
	}
	return nil
}

// Resume moves paused -> running and re-enables billing when stream is active.
func (s *Service) Resume(ctx context.Context, id string) error {
	sess, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if err := s.checkPrepaid(sess.AgentID); err != nil {
		if errors.Is(err, ErrInsufficientPrepaid) && s.hcs != nil {
			var rem int64
			if s.ledger != nil {
				rem, _ = s.ledger.GetBalance(sess.AgentID)
			}
			min := s.rateUnitsPerMinute
			if min <= 0 {
				min = 60
			}
			s.hcs.StartRejectedInsufficientPrepaid(ctx, id, sess.AgentID, min, rem)
		}
		return err
	}

	err = s.naryo.ResumeEgress(ctx, id)
	if err != nil {
		return err
	}
	err = s.store.Update(id, func(v *Session) (bool, error) {
		if v.State != StatePaused {
			return false, fmt.Errorf("%w: resume from %s", ErrInvalidTransition, v.State)
		}
		v.State = StateRunning
		v.PaymentStreamActive = true
		v.FundsChargeRetryPending = false
		v.FundsRetryAtMinute = 0
		v.AutoPausedForFunds = false
		return true, nil
	})
	if err != nil {
		return err
	}
	s.recordActivity("pipeline_resumed", id, nil)
	if s.hcs != nil {
		s.hcs.PipelineResumed(ctx, id)
	}
	return nil
}

// Reconfigure merges patch into the session config map (shallow per-key) and emits HCS.
func (s *Service) Reconfigure(ctx context.Context, id string, patch map[string]any) error {
	if len(patch) == 0 {
		_, err := s.store.Get(id)
		if err != nil {
			return err
		}
	} else {
		err := s.store.Update(id, func(v *Session) (bool, error) {
			if v.Config == nil {
				v.Config = make(map[string]any, len(patch))
			}
			for k, val := range patch {
				v.Config[k] = val
			}
			return true, nil
		})
		if err != nil {
			return err
		}
	}
	s.recordActivity("pipeline_reconfigured", id, map[string]any{"phase": 1})
	if s.hcs != nil {
		s.hcs.PipelineReconfigured(ctx, id, 1)
	}
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
	s.recordActivity("payment_stream", id, map[string]any{"active": active})
	if s.hcs != nil {
		if active {
			s.hcs.PaymentStreamStarted(ctx, id)
		} else {
			s.hcs.PaymentStreamStalled(ctx, id)
		}
	}
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

// ActivityForSession returns recent activity rows for one session (newest first).
func (s *Service) ActivityForSession(sessionID string, limit int) []ActivityEntry {
	if s == nil || s.activity == nil || sessionID == "" {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	const capScan = 512
	all := s.activity.Snapshot(capScan)
	out := make([]ActivityEntry, 0, limit)
	for _, e := range all {
		if e.SessionID == sessionID {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
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

// runBillingTick increments billed seconds; charges prepaid once per minute; emits batched HCS summaries.
func (s *Service) runBillingTick(ctx context.Context) {
	now := s.nowTime()
	curMin := now.Unix() / 60

	var pauseIDs []string

	for _, id := range s.store.IDs() {
		var (
			needPause          bool
			pauseRemainingBal  int64
			emitSummary        bool
			summaryArgs        BillingSummaryArgs
			summarySessionID   string
		)

		_ = s.store.Update(id, func(v *Session) (bool, error) {
			if v.State != StateRunning || !v.PaymentStreamActive {
				return false, nil
			}

			v.BilledSeconds++

			if s.hcs != nil && s.ledger == nil {
				s.hcs.BillingTick(ctx, id, v.BilledSeconds, v.RateCentsPerSecond)
			}

			if s.ledger == nil || v.AgentID == "" {
				return true, nil
			}

			rate := v.RateUnitsPerMinute
			if rate <= 0 {
				rate = s.rateUnitsPerMinute
			}
			if rate <= 0 {
				rate = 60
			}

			if v.LastPaidMinute == 0 {
				v.LastPaidMinute = curMin
				return true, nil
			}

			if v.LastPaidMinute >= curMin {
				return true, nil
			}

			if v.FundsChargeRetryPending && curMin < v.FundsRetryAtMinute {
				return true, nil
			}

			minuteToBill := v.LastPaidMinute
			rem, err := s.ledger.ChargeUsage(v.AgentID, id, rate, minuteToBill)
			if err != nil {
				if errors.Is(err, ledger.ErrInsufficientFunds) {
					if v.FundsChargeRetryPending {
						needPause = true
						pauseRemainingBal = rem
						v.State = StatePaused
						v.PaymentStreamActive = false
						v.AutoPausedForFunds = true
						v.FundsChargeRetryPending = false
						v.FundsRetryAtMinute = 0
						pauseIDs = append(pauseIDs, id)
						return true, nil
					}
					v.FundsChargeRetryPending = true
					v.FundsRetryAtMinute = curMin + 1
					return true, nil
				}
				return false, err
			}

			v.FundsChargeRetryPending = false
			v.FundsRetryAtMinute = 0
			v.LastPaidMinute++
			v.ChargedUnits += rate
			v.SummaryPendingUnits += rate
			v.SummaryPendingRuntimeSeconds += 60

			win := v.SummaryWindowMinutes
			if win < 5 {
				win = s.summaryWindowMinutes
			}
			win = clampSummaryWin(win)

			if v.LastSummaryEmittedAt.IsZero() {
				v.LastSummaryEmittedAt = now
			}
			if now.Sub(v.LastSummaryEmittedAt) >= time.Duration(win)*time.Minute && v.SummaryPendingUnits > 0 {
				emitSummary = true
				summarySessionID = id
				bal, _ := s.ledger.GetBalance(v.AgentID)
				summaryArgs = BillingSummaryArgs{
					AgentID:               v.AgentID,
					RuntimeSeconds:        v.SummaryPendingRuntimeSeconds,
					AmountChargedUnits:    v.SummaryPendingUnits,
					RemainingBalanceUnits: bal,
					SummaryWindowMinutes:  win,
				}
				v.LastSummaryEmittedAt = now
				v.SummaryPendingUnits = 0
				v.SummaryPendingRuntimeSeconds = 0
			}

			return true, nil
		})

		if needPause && s.hcs != nil {
			s.hcs.PipelinePaused(ctx, id, "insufficient_balance", pauseRemainingBal)
		}
		if emitSummary && s.hcs != nil {
			s.hcs.BillingSummary(ctx, summarySessionID, summaryArgs)
		}
	}

	for _, id := range pauseIDs {
		if err := s.naryo.PauseEgress(ctx, id); err != nil {
			// best-effort
			continue
		}
		s.recordActivity("pipeline_paused", id, map[string]any{"reason": "insufficient_balance"})
	}
}

func (s *Service) recordActivity(typ, sessionID string, data map[string]any) {
	if s.activity == nil {
		return
	}
	var cp map[string]any
	if len(data) > 0 {
		cp = make(map[string]any, len(data))
		for k, v := range data {
			cp[k] = v
		}
	}
	s.activity.Record(ActivityEntry{Type: typ, SessionID: sessionID, Data: cp})
}

// IngestNaryoEvent persists an inbound Naryo webhook payload for a pipeline session.
func (s *Service) IngestNaryoEvent(ctx context.Context, sessionID, eventID string, payload map[string]any) (duplicate bool, err error) {
	_ = ctx
	if s == nil {
		return false, errors.New("nil service")
	}
	dup, err := s.store.AppendNaryoEvent(sessionID, eventID, payload)
	if err != nil {
		return false, err
	}
	if !dup {
		s.recordActivity("naryo_event", sessionID, map[string]any{"eventId": eventID})
	}
	return dup, nil
}

// NaryoEventsForSession returns recent inbound Naryo events for a session.
func (s *Service) NaryoEventsForSession(sessionID string, limit int) ([]NaryoInboundEvent, error) {
	if s == nil {
		return nil, errors.New("nil service")
	}
	return s.store.NaryoEvents(sessionID, limit)
}
