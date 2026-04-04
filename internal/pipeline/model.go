package pipeline

import "time"

// State is the coarse lifecycle of a rented pipeline session.
type State string

const (
	StateCreated State = "created"
	StateRunning State = "running"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
)

// Session is an in-memory pipeline rental record.
type Session struct {
	ID                  string `json:"id"`
	AgentID             string `json:"agentId,omitempty"`
	State               State  `json:"state"`
	PaymentStreamActive bool   `json:"paymentStreamActive"`
	// BilledSeconds counts billing ticks while running and stream-active.
	BilledSeconds int64 `json:"billedSeconds"`
	// RateCentsPerSecond is logical billing rate used for HCS metadata (integer cents).
	RateCentsPerSecond int64 `json:"rateCentsPerSecond"`
	LastNaryoOpID      string `json:"lastNaryoOpId,omitempty"`
	// Config holds merged reconfigure patches (dashboard / agents).
	Config map[string]any `json:"config,omitempty"`

	// Prepaid / per-minute billing (off-chain ledger + batched HCS summaries).
	RateUnitsPerMinute           int64     `json:"rateUnitsPerMinute"`
	ChargedUnits                 int64     `json:"chargedUnits"`
	BillingNumerator             int64     `json:"billingNumerator,omitempty"`
	SecondsInDebitWindow         int64     `json:"secondsInDebitWindow,omitempty"`
	CommittedDebitSeq            int64     `json:"-"`
	FundsChargeRetryPending      bool      `json:"-"`
	FundsRetryAtUnix             int64     `json:"-"`
	SummaryPendingUnits          int64     `json:"-"`
	SummaryPendingRuntimeSeconds int64     `json:"-"`
	LastSummaryEmittedAt         time.Time `json:"-"`
	SummaryWindowMinutes         int64     `json:"summaryWindowMinutes"`
	AutoPausedForFunds           bool      `json:"autoPausedForFunds,omitempty"`
}
