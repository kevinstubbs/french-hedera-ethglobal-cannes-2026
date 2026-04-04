package ledger

import (
	"errors"
	"sync"
)

// ErrInsufficientFunds is returned when a debit would drop balance below zero.
var ErrInsufficientFunds = errors.New("insufficient prepaid balance")

// MemoryLedger tracks per-agent prepaid units (off-chain) with idempotent credits.
type MemoryLedger struct {
	mu       sync.Mutex
	balances map[string]int64 // agentID -> units
	credited map[string]struct{}
}

// NewMemoryLedger returns an empty ledger.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{
		balances: make(map[string]int64),
		credited: make(map[string]struct{}),
	}
}

func creditKey(agentID, sourceTxID, source string) string {
	return agentID + "|" + source + "|" + sourceTxID
}

// Credit adds units for an agent. When sourceTxID is non-empty, the same agent+source+sourceTxID is idempotent.
func (l *MemoryLedger) Credit(agentID string, amountUnits int64, sourceTxID string, source string) error {
	if l == nil {
		return nil
	}
	if agentID == "" || amountUnits <= 0 {
		return errors.New("invalid credit")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if sourceTxID != "" {
		key := creditKey(agentID, sourceTxID, source)
		if _, ok := l.credited[key]; ok {
			return nil
		}
		l.credited[key] = struct{}{}
	}
	l.balances[agentID] += amountUnits
	return nil
}

// GetBalance returns the current prepaid balance for an agent (0 if unknown).
func (l *MemoryLedger) GetBalance(agentID string) (int64, error) {
	if l == nil {
		return 0, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balances[agentID], nil
}

// CanAfford reports whether balance >= minUnits.
func (l *MemoryLedger) CanAfford(agentID string, minUnits int64) (bool, int64, error) {
	if l == nil {
		return true, 0, nil
	}
	bal, err := l.GetBalance(agentID)
	if err != nil {
		return false, 0, err
	}
	return bal >= minUnits, bal, nil
}

// ChargeUsage debits amountUnits for a pipeline minute bucket. Idempotent per (agentID, pipelineID, minuteBucket).
func (l *MemoryLedger) ChargeUsage(agentID, pipelineID string, amountUnits int64, minuteBucket int64) (remaining int64, err error) {
	if l == nil {
		return 0, nil
	}
	if agentID == "" || pipelineID == "" || amountUnits < 0 {
		return 0, errors.New("invalid charge")
	}
	if amountUnits == 0 {
		return l.GetBalance(agentID)
	}
	key := agentID + "|" + pipelineID + "|charge|" + itoa(minuteBucket)
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.credited[key]; ok {
		return l.balances[agentID], nil
	}
	bal := l.balances[agentID]
	if bal < amountUnits {
		return bal, ErrInsufficientFunds
	}
	l.balances[agentID] = bal - amountUnits
	l.credited[key] = struct{}{}
	return l.balances[agentID], nil
}

func itoa(v int64) string {
	// small fast path for minute buckets
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	s := string(buf[i:])
	if neg {
		return "-" + s
	}
	return s
}
