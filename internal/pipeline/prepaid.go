package pipeline

// PrepaidLedger is the off-chain per-agent balance used for pipeline entitlement.
type PrepaidLedger interface {
	Credit(agentID string, amountUnits int64, sourceTxID string, source string) error
	CanAfford(agentID string, minUnits int64) (ok bool, balance int64, err error)
	ChargeUsage(agentID, pipelineID string, amountUnits int64, minuteBucket int64) (remaining int64, err error)
	GetBalance(agentID string) (int64, error)
}

// BillingSummaryArgs is passed to HCS billing_summary emission.
type BillingSummaryArgs struct {
	AgentID               string
	RuntimeSeconds        int64
	AmountChargedUnits    int64
	RemainingBalanceUnits int64
	SummaryWindowMinutes  int64
	ConfigHash            string
}

// AgentTopUpArgs is passed to HCS agent_top_up emission.
type AgentTopUpArgs struct {
	AgentID     string
	AmountUnits int64
	Source      string
	SourceTxID  string
	Asset       string
}
