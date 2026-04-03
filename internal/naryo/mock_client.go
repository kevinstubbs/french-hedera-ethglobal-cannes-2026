package naryo

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// MockClient records calls and returns deterministic operation IDs.
type MockClient struct {
	EnsureCalls int32
	PauseCalls  int32
	ResumeCalls int32
	StopCalls   int32

	opSeq int64
}

// EnsurePipeline implements Client.
func (m *MockClient) EnsurePipeline(ctx context.Context, sessionID string) (string, error) {
	if m == nil {
		return "", nil
	}
	atomic.AddInt32(&m.EnsureCalls, 1)
	id := atomic.AddInt64(&m.opSeq, 1)
	return fmt.Sprintf("mock-op-ensure-%d", id), nil
}

// PauseEgress implements Client.
func (m *MockClient) PauseEgress(ctx context.Context, sessionID string) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.PauseCalls, 1)
	return nil
}

// ResumeEgress implements Client.
func (m *MockClient) ResumeEgress(ctx context.Context, sessionID string) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.ResumeCalls, 1)
	return nil
}

// StopPipeline implements Client.
func (m *MockClient) StopPipeline(ctx context.Context, sessionID string) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.StopCalls, 1)
	return nil
}

// Stats returns mock adapter call counts for observability.
func (m *MockClient) Stats() map[string]any {
	if m == nil {
		return map[string]any{"mode": "nil"}
	}
	return map[string]any{
		"mode":         "mock",
		"healthy":      true,
		"lastChecked":  time.Now().UTC().Format(time.RFC3339Nano),
		"ensureCalls":  atomic.LoadInt32(&m.EnsureCalls),
		"pauseCalls":   atomic.LoadInt32(&m.PauseCalls),
		"resumeCalls":  atomic.LoadInt32(&m.ResumeCalls),
		"stopCalls":    atomic.LoadInt32(&m.StopCalls),
		"lastOpSeq":    atomic.LoadInt64(&m.opSeq),
	}
}
