package hcs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// Clock supplies time for envelopes (tests use a fixed clock).
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Logger emits one JSON line per event using [Envelope].
type Logger struct {
	log            *slog.Logger
	clock          Clock
	topicSubmitter TopicSubmitter
	topicID        string
	hcsQueue       chan []byte
}

// LoggerOption configures [NewLogger].
type LoggerOption func(*Logger)

// WithClock sets a non-default clock (for tests).
func WithClock(c Clock) LoggerOption {
	return func(l *Logger) {
		if c != nil {
			l.clock = c
		}
	}
}

// WithSlog sets the slog sink (defaults to slog.Default()).
func WithSlog(log *slog.Logger) LoggerOption {
	return func(l *Logger) {
		if log != nil {
			l.log = log
		}
	}
}

// WithHCSTopic enables async best-effort submit of each envelope JSON to an HCS topic.
func WithHCSTopic(sub TopicSubmitter, topicID string) LoggerOption {
	return func(l *Logger) {
		if sub == nil || topicID == "" {
			return
		}
		l.topicSubmitter = sub
		l.topicID = topicID
		l.hcsQueue = make(chan []byte, 256)
	}
}

// NewLogger builds an HCS JSON-line logger.
func NewLogger(opts ...LoggerOption) *Logger {
	l := &Logger{
		log:   slog.Default(),
		clock: systemClock{},
	}
	for _, o := range opts {
		o(l)
	}
	if l.hcsQueue != nil && l.topicSubmitter != nil {
		go l.hcsSubmitLoop()
	}
	return l
}

func (l *Logger) emit(ctx context.Context, env Envelope) {
	if l == nil {
		return
	}
	b, err := json.Marshal(env)
	if err != nil {
		slog.Error("hcs: marshal envelope", "err", err)
		return
	}
	l.log.InfoContext(ctx, string(b))
	l.enqueueHCSPayload(b)
}

func (l *Logger) envelope(eventType, sessionID string, payload json.RawMessage) Envelope {
	clock := Clock(systemClock{})
	if l != nil && l.clock != nil {
		clock = l.clock
	}
	ts := clock.Now()
	return Envelope{
		SchemaVersion: SchemaVersion,
		EventType:     eventType,
		Timestamp:     FormatTimestamp(ts),
		SessionID:     sessionID,
		Payload:       payload,
	}
}

// PipelineCreated emits pipeline_created.
func (l *Logger) PipelineCreated(ctx context.Context, sessionID, agentID string) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadPipelineCreated{AgentID: agentID})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventPipelineCreated, sessionID, p))
}

// PipelineStarted emits pipeline_started.
func (l *Logger) PipelineStarted(ctx context.Context, sessionID, agentID, naryoOpID string) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadPipelineStarted{AgentID: agentID, NaryoOpID: naryoOpID})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventPipelineStarted, sessionID, p))
}

var emptyPayload = json.RawMessage(`{}`)

// PipelinePaused emits pipeline_paused.
func (l *Logger) PipelinePaused(ctx context.Context, sessionID, reason string, remainingBalanceUnits int64) {
	if l == nil {
		return
	}
	if reason == "" && remainingBalanceUnits == 0 {
		l.emit(ctx, l.envelope(EventPipelinePaused, sessionID, emptyPayload))
		return
	}
	p, err := MarshalPayload(PayloadPipelinePaused{Reason: reason, RemainingBalanceUnits: remainingBalanceUnits})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventPipelinePaused, sessionID, p))
}

// PipelineResumed emits pipeline_resumed.
func (l *Logger) PipelineResumed(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPipelineResumed, sessionID, emptyPayload))
}

// PipelineStopped emits pipeline_stopped.
func (l *Logger) PipelineStopped(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPipelineStopped, sessionID, emptyPayload))
}

// PipelineReconfigured emits pipeline_reconfigured.
func (l *Logger) PipelineReconfigured(ctx context.Context, sessionID string, phase int) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadPipelineReconfigured{Phase: phase})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventPipelineReconfigured, sessionID, p))
}

// BillingTick emits billing_tick.
func (l *Logger) BillingTick(ctx context.Context, sessionID string, billedSeconds, rateCentsPerSecond int64) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadBillingTick{
		BilledSeconds:      billedSeconds,
		RateCentsPerSecond: rateCentsPerSecond,
	})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventBillingTick, sessionID, p))
}

// PaymentStreamStarted emits payment_stream_started.
func (l *Logger) PaymentStreamStarted(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPaymentStreamStarted, sessionID, emptyPayload))
}

// PaymentStreamStalled emits payment_stream_stalled (payment inactive while session may still run).
func (l *Logger) PaymentStreamStalled(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPaymentStreamStalled, sessionID, emptyPayload))
}

// PaymentStreamTerminated emits payment_stream_terminated (stream ended with session shutdown path).
func (l *Logger) PaymentStreamTerminated(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPaymentStreamTerminated, sessionID, emptyPayload))
}

// BillingSummary emits billing_summary.
func (l *Logger) BillingSummary(ctx context.Context, sessionID string, args pipeline.BillingSummaryArgs) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadBillingSummary{
		PipelineID:            sessionID,
		AgentID:               args.AgentID,
		RuntimeSeconds:        args.RuntimeSeconds,
		AmountChargedUnits:    args.AmountChargedUnits,
		RemainingBalanceUnits: args.RemainingBalanceUnits,
		SummaryWindowMinutes:  args.SummaryWindowMinutes,
		ConfigHash:            args.ConfigHash,
	})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventBillingSummary, sessionID, p))
}

// AgentTopUp emits agent_top_up (sessionId empty at envelope level).
func (l *Logger) AgentTopUp(ctx context.Context, args pipeline.AgentTopUpArgs) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadAgentTopUp{
		AgentID:     args.AgentID,
		AmountUnits: args.AmountUnits,
		Source:      args.Source,
		SourceTxID:  args.SourceTxID,
		Asset:       args.Asset,
	})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventAgentTopUp, "", p))
}

// StartRejectedInsufficientPrepaid emits start_rejected_insufficient_prepaid.
func (l *Logger) StartRejectedInsufficientPrepaid(ctx context.Context, sessionID, agentID string, requiredUnits, remainingUnits int64) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadStartRejectedInsufficient{
		AgentID:               agentID,
		RequiredUnits:         requiredUnits,
		RemainingBalanceUnits: remainingUnits,
	})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventStartRejectedInsufficient, sessionID, p))
}
