// Package naryo is the orchestrator adapter for Naryo’s Configuration API ([HTTPClient]).
//
// Target production topology: Naryo HTTP broadcasters send only to this platform’s ingest
// endpoints; the API persists events and optionally POSTs to per-pipeline agent webhooks.
// Agents without webhooks query the API for historical events. See docs/PIPELINE_EVENT_ROUTING.md.
package naryo

import "context"

// Client is the minimal Configuration API surface the orchestrator needs.
type Client interface {
	EnsurePipeline(ctx context.Context, in EnsurePipelineArgs) (operationID string, err error)
	PauseEgress(ctx context.Context, sessionID string) error
	ResumeEgress(ctx context.Context, in EnsurePipelineArgs) error
	StopPipeline(ctx context.Context, sessionID string) error
	Stats() map[string]any
	// ConfigurationSnapshot returns a read-only view from Naryo’s Configuration API (GET /filters, /broadcasters, …).
	// Used for debugging; [RecordingClient] (tests only) returns a stub without calling Naryo.
	ConfigurationSnapshot(ctx context.Context) (map[string]any, error)
}
