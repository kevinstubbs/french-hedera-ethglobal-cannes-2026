package naryo

import "context"

// Client is the minimal Configuration API surface the orchestrator needs (Phase 1 mock).
type Client interface {
	EnsurePipeline(ctx context.Context, sessionID string) (operationID string, err error)
	PauseEgress(ctx context.Context, sessionID string) error
	ResumeEgress(ctx context.Context, sessionID string) error
	StopPipeline(ctx context.Context, sessionID string) error
}
