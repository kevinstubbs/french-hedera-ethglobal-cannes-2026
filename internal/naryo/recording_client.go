package naryo

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// RecordingClient implements [Client] for tests: it records calls and does not contact Naryo.
// Production uses [NewFromEnv] → [HTTPClient] only.
type RecordingClient struct {
	EnsureCalls int32
	PauseCalls  int32
	ResumeCalls int32
	StopCalls   int32

	opSeq int64
}

// EnsurePipeline implements Client.
func (m *RecordingClient) EnsurePipeline(ctx context.Context, in EnsurePipelineArgs) (string, error) {
	if m == nil {
		return "", nil
	}
	atomic.AddInt32(&m.EnsureCalls, 1)
	id := atomic.AddInt64(&m.opSeq, 1)
	return fmt.Sprintf("recording-op-ensure-%d", id), nil
}

// PauseEgress implements Client.
func (m *RecordingClient) PauseEgress(ctx context.Context, sessionID string) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.PauseCalls, 1)
	return nil
}

// ResumeEgress implements Client.
func (m *RecordingClient) ResumeEgress(ctx context.Context, in EnsurePipelineArgs) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.ResumeCalls, 1)
	return nil
}

// StopPipeline implements Client.
func (m *RecordingClient) StopPipeline(ctx context.Context, sessionID string) error {
	if m == nil {
		return nil
	}
	atomic.AddInt32(&m.StopCalls, 1)
	return nil
}

// ConfigurationSnapshot implements Client (no live Naryo).
func (m *RecordingClient) ConfigurationSnapshot(ctx context.Context) (map[string]any, error) {
	_ = ctx
	return map[string]any{
		"mode":                           "recording",
		"message":                        "In-process recording client (Go tests only; does not call Naryo)",
		"filters":                        []any{},
		"filtersCount":                   0,
		"broadcasters":                   []any{},
		"broadcastersCount":              0,
		"broadcasterConfigurations":      []any{},
		"broadcasterConfigurationsCount": 0,
		"egressProvisionSkipped":         false,
		"orchestratorSessions":           []any{},
		"orchestratorSessionsCount":      0,
		"snapshotHints": []any{
			"Recording client: used in Go tests only. The API binary always requires NARYO_CONFIG_API_BASE and NARYO_PLATFORM_INGEST_URL.",
		},
	}, nil
}

// Stats implements Client.
func (m *RecordingClient) Stats() map[string]any {
	if m == nil {
		return map[string]any{"mode": "nil"}
	}
	return map[string]any{
		"mode":        "recording",
		"healthy":     true,
		"lastChecked": time.Now().UTC().Format(time.RFC3339Nano),
		"ensureCalls": atomic.LoadInt32(&m.EnsureCalls),
		"pauseCalls":  atomic.LoadInt32(&m.PauseCalls),
		"resumeCalls": atomic.LoadInt32(&m.ResumeCalls),
		"stopCalls":   atomic.LoadInt32(&m.StopCalls),
		"lastOpSeq":   atomic.LoadInt64(&m.opSeq),
	}
}
