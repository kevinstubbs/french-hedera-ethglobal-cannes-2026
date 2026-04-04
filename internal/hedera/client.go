package hedera

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// Client verifies on-chain top-ups and submits HCS messages (HTS/HCS only; no contracts).
type Client interface {
	VerifyTopupTx(ctx context.Context, txID string, expectedToAccount string, expectedAgentID string) (amountUnits int64, asset string, err error)
	SubmitHCSMessage(ctx context.Context, topicID string, payload []byte) (submitTxID string, err error)
}

// NoopClient implements [Client] with no-op verification and logging-only submit.
type NoopClient struct{}

func (NoopClient) VerifyTopupTx(_ context.Context, txID, expectedToAccount, expectedAgentID string) (int64, string, error) {
	if strings.TrimSpace(txID) == "" {
		return 0, "", errors.New("hedera: empty transaction id")
	}
	if strings.TrimSpace(expectedToAccount) == "" {
		return 0, "", errors.New("hedera: service account not configured")
	}
	_ = expectedAgentID
	return 0, "", errors.New("hedera: VerifyTopupTx not configured (set HEDERA_ENABLED and operator credentials)")
}

func (NoopClient) SubmitHCSMessage(_ context.Context, topicID string, payload []byte) (string, error) {
	slog.Debug("hedera noop SubmitHCSMessage", "topicId", topicID, "bytes", len(payload))
	return "", errors.New("hedera: HCS submit not configured")
}

// NewClientFromConfig returns a live client when enabled and configured, else [NoopClient].
func NewClientFromConfig(cfg config.Hedera) Client {
	if !cfg.Enabled || cfg.OperatorAccountID == "" || cfg.OperatorPrivateKey == "" {
		return NoopClient{}
	}
	return newLiveClient(cfg)
}
