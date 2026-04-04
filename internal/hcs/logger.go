package hcs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Clock supplies time for envelopes (tests use a fixed clock).
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Logger emits one JSON line per event using [Envelope].
type Logger struct {
	log   *slog.Logger
	clock Clock
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

// NewLogger builds an HCS JSON-line logger.
func NewLogger(opts ...LoggerOption) *Logger {
	l := &Logger{
		log:   slog.Default(),
		clock: systemClock{},
	}
	for _, o := range opts {
		o(l)
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
func (l *Logger) PipelineStarted(ctx context.Context, sessionID, naryoOpID string) {
	if l == nil {
		return
	}
	p, err := MarshalPayload(PayloadPipelineStarted{NaryoOpID: naryoOpID})
	if err != nil {
		slog.Error("hcs: payload", "err", err)
		return
	}
	l.emit(ctx, l.envelope(EventPipelineStarted, sessionID, p))
}

var emptyPayload = json.RawMessage(`{}`)

// PipelinePaused emits pipeline_paused.
func (l *Logger) PipelinePaused(ctx context.Context, sessionID string) {
	if l == nil {
		return
	}
	l.emit(ctx, l.envelope(EventPipelinePaused, sessionID, emptyPayload))
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
