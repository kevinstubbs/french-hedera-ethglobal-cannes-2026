package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/ledger"
)

func TestBillingTickIncrementsPerSecondWindow(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 3, nil)
	sess, err := svc.Create(ctx, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		svc.BillingTick(ctx)
	}
	got, err := svc.Status(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BilledSeconds != 5 {
		t.Fatalf("expected 5 billing ticks, got %d", got.BilledSeconds)
	}
	if got.RateCentsPerSecond != 3 {
		t.Fatalf("rate: got %d", got.RateCentsPerSecond)
	}
}

func TestBillingSkipsWhenPaymentStreamInactive(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 1, nil)
	sess, _ := svc.Create(ctx, "")
	if err := svc.Start(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	svc.BillingTick(ctx)
	if err := svc.SetPaymentStreamActive(ctx, sess.ID, false); err != nil {
		t.Fatal(err)
	}
	svc.BillingTick(ctx)
	svc.BillingTick(ctx)
	got, _ := svc.Status(sess.ID)
	if got.BilledSeconds != 1 {
		t.Fatalf("expected 1 tick before stream paused, got %d", got.BilledSeconds)
	}
}

func TestBillingSkipsWhenNotRunning(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 1, nil)
	sess, _ := svc.Create(ctx, "")
	svc.BillingTick(ctx)
	got, _ := svc.Status(sess.ID)
	if got.BilledSeconds != 0 {
		t.Fatalf("created session should not bill, got %d", got.BilledSeconds)
	}
	_ = sess
}

func TestPauseStopsBillingResumeRestarts(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 1, nil)
	sess, _ := svc.Create(ctx, "")
	_ = svc.Start(ctx, sess.ID)
	svc.BillingTick(ctx)
	if err := svc.Pause(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	svc.BillingTick(ctx)
	if err := svc.Resume(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	svc.BillingTick(ctx)
	got, _ := svc.Status(sess.ID)
	if got.BilledSeconds != 2 {
		t.Fatalf("expected 2 ticks (before pause + after resume), got %d", got.BilledSeconds)
	}
	if got.State != StateRunning {
		t.Fatalf("state: %s", got.State)
	}
}

// mockNaryo is a minimal naryo.Client for pipeline package tests.
type mockNaryo struct{}

func (mockNaryo) EnsurePipeline(ctx context.Context, sessionID string) (string, error) {
	return "op-" + sessionID, nil
}
func (mockNaryo) PauseEgress(ctx context.Context, sessionID string) error   { return nil }
func (mockNaryo) ResumeEgress(ctx context.Context, sessionID string) error { return nil }
func (mockNaryo) StopPipeline(ctx context.Context, sessionID string) error  { return nil }
func (mockNaryo) Stats() map[string]any {
	return map[string]any{"mode": "mockNaryo"}
}

func TestPrepaidPeriodicDebit(t *testing.T) {
	ctx := context.Background()
	tick := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return tick }

	led := ledger.NewMemoryLedger()
	if err := led.Credit("agent-1", 10_000, "", "seed"); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 1, nil,
		WithPrepaidLedger(led),
		WithRateUnitsPerMinute(60),
		WithDebitIntervalSeconds(60),
		WithSummaryWindowMinutes(5),
		WithClock(clock),
	)
	sess, err := svc.Create(ctx, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	for range 59 {
		svc.BillingTick(ctx)
	}
	bal, _ := led.GetBalance("agent-1")
	if bal != 10_000 {
		t.Fatalf("before first debit window want 10000 got %d", bal)
	}
	svc.BillingTick(ctx)
	bal, _ = led.GetBalance("agent-1")
	// 60 seconds at 60 units/min: numerator 3600 -> 60 units charged
	if bal != 10_000-60 {
		t.Fatalf("after first window want %d got %d", 10_000-60, bal)
	}
}

func TestPrepaidBillingSummaryWindow(t *testing.T) {
	ctx := context.Background()
	tick := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return tick }
	h := &mockHCS{}
	led := ledger.NewMemoryLedger()
	_ = led.Credit("a1", 100_000, "", "seed")
	svc := NewService(NewMemoryStore(), &mockNaryo{}, h, 1, nil,
		WithPrepaidLedger(led),
		WithRateUnitsPerMinute(1),
		WithDebitIntervalSeconds(60),
		WithSummaryWindowMinutes(5),
		WithClock(clock),
	)
	sess, _ := svc.Create(ctx, "a1")
	_ = svc.Start(ctx, sess.ID)
	// First debit after 60 ticks at t0 (1 unit).
	for range 60 {
		svc.BillingTick(ctx)
	}
	// Advance wall clock past summary window, then second debit triggers billing_summary.
	tick = tick.Add(6 * time.Minute)
	for range 60 {
		svc.BillingTick(ctx)
	}
	if h.summaries < 1 {
		t.Fatalf("expected at least one billing_summary, got %d", h.summaries)
	}
}

func TestCreateRejectedWithoutBalance(t *testing.T) {
	ctx := context.Background()
	led := ledger.NewMemoryLedger()
	svc := NewService(NewMemoryStore(), &mockNaryo{}, nil, 1, nil,
		WithPrepaidLedger(led),
		WithRateUnitsPerMinute(1),
		WithMinStartMinutes(10),
	)
	_, err := svc.Create(ctx, "b1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInsufficientPrepaid) {
		t.Fatalf("want ErrInsufficientPrepaid got %v", err)
	}
}
