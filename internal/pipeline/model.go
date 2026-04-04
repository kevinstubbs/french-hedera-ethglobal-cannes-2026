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

	// Prepaid / per-minute billing (off-chain ledger + batched HCS summaries).
	RateUnitsPerMinute           int64     `json:"rateUnitsPerMinute"`
	ChargedUnits                 int64     `json:"chargedUnits"`
	LastPaidMinute               int64     `json:"lastPaidMinute,omitempty"`
	FundsChargeRetryPending      bool      `json:"-"`
	FundsRetryAtMinute           int64     `json:"-"`
	SummaryPendingUnits          int64     `json:"-"`
	SummaryPendingRuntimeSeconds int64     `json:"-"`
	LastSummaryEmittedAt         time.Time `json:"-"`
	SummaryWindowMinutes         int64     `json:"summaryWindowMinutes"`
	AutoPausedForFunds           bool      `json:"autoPausedForFunds,omitempty"`
}
