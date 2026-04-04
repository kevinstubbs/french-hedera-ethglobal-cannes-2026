package hcs

import (
	"encoding/json"
	"time"
)

// SchemaVersion is bumped when envelope or payload shapes change incompatibly.
const SchemaVersion = "1"

// Canonical event types for HCS / mirror logging (stable identifiers).
const (
	EventPipelineCreated      = "pipeline_created"
	EventPipelineStarted      = "pipeline_started"
	EventPipelinePaused       = "pipeline_paused"
	EventPipelineResumed      = "pipeline_resumed"
	EventPipelineStopped      = "pipeline_stopped"
	EventPipelineReconfigured = "pipeline_reconfigured"
	EventBillingTick             = "billing_tick"
	EventPaymentStreamStarted    = "payment_stream_started"
	EventPaymentStreamStalled    = "payment_stream_stalled"
	EventPaymentStreamTerminated = "payment_stream_terminated"
)

// Envelope is the on-wire JSON shape for every HCS-oriented log line.
// Struct field order defines JSON key order for stable, deterministic encoding.
type Envelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	EventType     string          `json:"eventType"`
	Timestamp     string          `json:"timestamp"`
	SessionID     string          `json:"sessionId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// Payload types (stable field sets for deterministic payload JSON).

type PayloadPipelineCreated struct {
	AgentID string `json:"agentId"`
}

type PayloadPipelineStarted struct {
	NaryoOpID string `json:"naryoOpId"`
}

type PayloadPipelineReconfigured struct {
	Phase int `json:"phase"`
}

type PayloadBillingTick struct {
	BilledSeconds      int64 `json:"billedSeconds"`
	RateCentsPerSecond int64 `json:"rateCentsPerSecond"`
}

// PayloadPaymentStream* use empty JSON objects {} when there is no extra data.

// FormatTimestamp returns UTC RFC3339Nano with trailing zeros trimmed consistently.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// MarshalPayload encodes a payload value to compact JSON (deterministic key order from struct fields).
func MarshalPayload(v any) (json.RawMessage, error) {
	return json.Marshal(v)
}
