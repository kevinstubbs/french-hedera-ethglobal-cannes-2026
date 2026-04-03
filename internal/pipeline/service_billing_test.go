package pipeline

import (
	"context"
	"testing"
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
