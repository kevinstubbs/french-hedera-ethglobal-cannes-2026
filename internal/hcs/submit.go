package hcs

import (
	"context"
	"log/slog"
)

// TopicSubmitter submits serialized envelope bytes to an HCS topic (best-effort).
type TopicSubmitter interface {
	SubmitHCSMessage(ctx context.Context, topicID string, payload []byte) (submitTxID string, err error)
}

func (l *Logger) hcsSubmitLoop() {
	for payload := range l.hcsQueue {
		if l.topicSubmitter == nil {
			continue
		}
		ctx := context.Background()
		txID, err := l.topicSubmitter.SubmitHCSMessage(ctx, l.topicID, payload)
		if err != nil {
			slog.Warn("hcs: topic submit failed", "err", err)
			continue
		}
		if txID != "" {
			slog.Debug("hcs: topic submit ok", "transactionId", txID, "bytes", len(payload))
		}
	}
}

func (l *Logger) enqueueHCSPayload(b []byte) {
	if l == nil || l.hcsQueue == nil {
		return
	}
	select {
	case l.hcsQueue <- b:
	default:
		slog.Warn("hcs: submit queue full, dropping message", "bytes", len(b))
	}
}
