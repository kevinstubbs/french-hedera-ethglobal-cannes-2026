package config

import (
	"os"
	"strconv"
	"strings"
)

// Hedera holds optional Hedera/HCS integration settings.
type Hedera struct {
	Enabled              bool
	Network              string // e.g. testnet
	OperatorAccountID    string
	OperatorPrivateKey   string
	ServiceAccountID     string // expected recipient for top-up verification
	AuditTopicID         string // HCS topic for async submit
	SummaryWindowMinutes int64  // 5–15, default 10
	// SkipTopupVerify allows crediting deposit top-ups without a successful VerifyTopupTx (demo only).
	SkipTopupVerify bool
}

// LoadHederaFromEnv reads Hedera-related environment variables.
func LoadHederaFromEnv() Hedera {
	h := Hedera{
		Network:              getEnvDefault("HEDERA_NETWORK", "testnet"),
		OperatorAccountID:    strings.TrimSpace(os.Getenv("HEDERA_OPERATOR_ID")),
		OperatorPrivateKey:   strings.TrimSpace(os.Getenv("HEDERA_OPERATOR_KEY")),
		ServiceAccountID:     strings.TrimSpace(os.Getenv("HEDERA_SERVICE_ACCOUNT_ID")),
		AuditTopicID:         strings.TrimSpace(os.Getenv("HEDERA_AUDIT_TOPIC_ID")),
		SummaryWindowMinutes: 10,
	}
	if v := strings.TrimSpace(os.Getenv("HEDERA_ENABLED")); v != "" {
		h.Enabled = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	} else {
		h.Enabled = h.AuditTopicID != "" && h.OperatorAccountID != "" && h.OperatorPrivateKey != ""
	}
	if v := strings.TrimSpace(os.Getenv("HEDERA_SUMMARY_WINDOW_MINUTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			h.SummaryWindowMinutes = clampSummaryWindow(n)
		}
	} else {
		h.SummaryWindowMinutes = clampSummaryWindow(h.SummaryWindowMinutes)
	}
	if v := strings.TrimSpace(os.Getenv("HEDERA_SKIP_TOPUP_VERIFY")); v != "" {
		h.SkipTopupVerify = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	return h
}

func clampSummaryWindow(n int64) int64 {
	switch {
	case n < 5:
		return 5
	case n > 15:
		return 15
	default:
		return n
	}
}

func getEnvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
