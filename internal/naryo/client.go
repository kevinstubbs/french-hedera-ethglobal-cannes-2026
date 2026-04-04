// Package naryo is the orchestrator adapter for Naryo’s Configuration API (currently mocked).
//
// Target production topology: Naryo HTTP broadcasters send only to this platform’s ingest
// endpoints; the API persists events and optionally POSTs to per-pipeline agent webhooks.
// Agents without webhooks query the API for historical events. See docs/PIPELINE_EVENT_ROUTING.md.
package naryo

import "context"

// Client is the minimal Configuration API surface the orchestrator needs (Phase 1 mock).
type Client interface {
	EnsurePipeline(ctx context.Context, sessionID string) (operationID string, err error)
	PauseEgress(ctx context.Context, sessionID string) error
	ResumeEgress(ctx context.Context, sessionID string) error
	StopPipeline(ctx context.Context, sessionID string) error
}
