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
	EventBillingSummary          = "billing_summary"
	EventAgentTopUp              = "agent_top_up"
	EventStartRejectedInsufficient = "start_rejected_insufficient_prepaid"
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
	AgentID   string `json:"agentId,omitempty"`
	NaryoOpID string `json:"naryoOpId"`
}

type PayloadPipelineReconfigured struct {
	Phase int `json:"phase"`
}

type PayloadBillingTick struct {
	BilledSeconds      int64 `json:"billedSeconds"`
	RateCentsPerSecond int64 `json:"rateCentsPerSecond"`
}

// PayloadBillingSummary batches per-minute charges for HCS audit.
type PayloadBillingSummary struct {
	PipelineID            string `json:"pipelineId"`
	AgentID               string `json:"agentId"`
	RuntimeSeconds        int64  `json:"runtimeSeconds"`
	AmountChargedUnits    int64  `json:"amountChargedUnits"`
	RemainingBalanceUnits int64  `json:"remainingBalanceUnits"`
	SummaryWindowMinutes  int64  `json:"summaryWindowMinutes"`
	ConfigHash            string `json:"configHash,omitempty"`
}

type PayloadAgentTopUp struct {
	AgentID      string `json:"agentId"`
	AmountUnits  int64  `json:"amountUnits"`
	Source       string `json:"source"`
	SourceTxID   string `json:"sourceTxId,omitempty"`
	Asset        string `json:"asset,omitempty"`
}

type PayloadStartRejectedInsufficient struct {
	AgentID               string `json:"agentId"`
	RequiredUnits         int64  `json:"requiredUnits"`
	RemainingBalanceUnits int64  `json:"remainingBalanceUnits"`
}

type PayloadPipelinePaused struct {
	Reason                string `json:"reason,omitempty"`
	RemainingBalanceUnits int64  `json:"remainingBalanceUnits,omitempty"`
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
