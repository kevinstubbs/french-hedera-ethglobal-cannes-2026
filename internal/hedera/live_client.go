package hedera

import (
	"context"
	"fmt"
	"strings"

	hiero "github.com/hiero-ledger/hiero-sdk-go/v2/sdk"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

type liveClient struct {
	client *hiero.Client
	cfg    config.Hedera
}

func newLiveClient(cfg config.Hedera) Client {
	var c *hiero.Client
	switch strings.ToLower(strings.TrimSpace(cfg.Network)) {
	case "mainnet":
		c = hiero.ClientForMainnet()
	case "previewnet":
		c = hiero.ClientForPreviewnet()
	default:
		c = hiero.ClientForTestnet()
	}

	operatorID, err := hiero.AccountIDFromString(cfg.OperatorAccountID)
	if err != nil {
		return errClient{err: fmt.Errorf("hedera operator id: %w", err)}
	}
	key, err := hiero.PrivateKeyFromString(cfg.OperatorPrivateKey)
	if err != nil {
		return errClient{err: fmt.Errorf("hedera operator key: %w", err)}
	}

	c.SetOperator(operatorID, key)
	return &liveClient{client: c, cfg: cfg}
}

type errClient struct {
	err error
}

func (e errClient) VerifyTopupTx(context.Context, string, string, string) (int64, string, error) {
	return 0, "", e.err
}

func (e errClient) SubmitHCSMessage(context.Context, string, []byte) (string, error) {
	return "", e.err
}

func (l *liveClient) VerifyTopupTx(ctx context.Context, txID, expectedToAccount, expectedAgentID string) (int64, string, error) {
	txID = strings.TrimSpace(txID)
	if txID == "" {
		return 0, "", fmt.Errorf("hedera: empty transaction id")
	}
	expectedToAccount = strings.TrimSpace(expectedToAccount)
	if expectedToAccount == "" {
		return 0, "", fmt.Errorf("hedera: HEDERA_SERVICE_ACCOUNT_ID not set")
	}

	tid, err := hiero.TransactionIdFromString(txID)
	if err != nil {
		return 0, "", fmt.Errorf("hedera: parse transaction id: %w", err)
	}
	to, err := hiero.AccountIDFromString(expectedToAccount)
	if err != nil {
		return 0, "", fmt.Errorf("hedera: parse service account: %w", err)
	}

	record, err := hiero.NewTransactionRecordQuery().
		SetTransactionID(tid).
		Execute(l.client)
	if err != nil {
		return 0, "", fmt.Errorf("hedera: transaction record: %w", err)
	}

	if expectedAgentID != "" && !strings.Contains(record.TransactionMemo, expectedAgentID) {
		return 0, "", fmt.Errorf("hedera: transaction memo does not reference agentId")
	}

	var creditedTinybar int64
	for _, tr := range record.Transfers {
		if tr.AccountID.Compare(to) == 0 {
			tb := tr.Amount.AsTinybar()
			if tb > 0 {
				creditedTinybar += tb
			}
		}
	}

	if creditedTinybar == 0 {
		for _, transfers := range record.TokenTransfers {
			for _, x := range transfers {
				if x.AccountID.Compare(to) == 0 && x.Amount > 0 {
					creditedTinybar += x.Amount
				}
			}
		}
	}

	if creditedTinybar == 0 {
		return 0, "", fmt.Errorf("hedera: no inbound transfer to service account in record")
	}

	return creditedTinybar, "HBAR_OR_TOKEN", nil
}

func (l *liveClient) SubmitHCSMessage(ctx context.Context, topicID string, payload []byte) (string, error) {
	topicID = strings.TrimSpace(topicID)
	if topicID == "" {
		return "", fmt.Errorf("hedera: empty topic id")
	}
	tid, err := hiero.TopicIDFromString(topicID)
	if err != nil {
		return "", fmt.Errorf("hedera: topic id: %w", err)
	}
	resp, err := hiero.NewTopicMessageSubmitTransaction().
		SetTopicID(tid).
		SetMessage(payload).
		Execute(l.client)
	if err != nil {
		return "", err
	}
	_ = ctx
	return resp.TransactionID.String(), nil
}
