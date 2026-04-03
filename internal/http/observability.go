package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// ObservabilityDeps are wired in main (not payment-gated).
type ObservabilityDeps struct {
	Svc   *pipeline.Service
	Naryo interface{ Stats() map[string]any }
}

// ObservabilitySummary serves GET /observability/v1/summary
func ObservabilitySummary(deps *ObservabilityDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessions := deps.Svc.ListSessions()
		activity := deps.Svc.ActivityFeed(100)

		var naryo map[string]any
		if deps.Naryo != nil {
			naryo = deps.Naryo.Stats()
		} else {
			naryo = map[string]any{"healthy": false, "message": "naryo client not configured"}
		}

		payments := summarizePayments(sessions, activity)

		payload := map[string]any{
			"generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"api": map[string]any{
				"status": "ok",
			},
			"pipelines": sessions,
			"activity":  activity,
			"naryo":     naryo,
			"payments":  payments,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func summarizePayments(sessions []pipeline.Session, activity []pipeline.ActivityEntry) map[string]any {
	var runningPaid, streamOn int
	var estCents int64
	for _, s := range sessions {
		if s.State == pipeline.StateRunning {
			runningPaid++
			if s.PaymentStreamActive {
				streamOn++
			}
			estCents += s.BilledSeconds * s.RateCentsPerSecond
		}
	}
	return map[string]any{
		"x402": map[string]any{
			"note": "Paid mutations use Coinbase x402 (PAYMENT-SIGNATURE). This summary infers state from sessions + activity only.",
		},
		"runningPipelines":       runningPaid,
		"streamsActive":          streamOn,
		"estimatedBilledCents":   estCents,
		"recentPaymentEvents": countTypes(activity, []string{"payment_stream", "pipeline_created", "pipeline_started"}),
	}
}

func countTypes(activity []pipeline.ActivityEntry, types []string) int {
	want := make(map[string]bool, len(types))
	for _, t := range types {
		want[t] = true
	}
	n := 0
	for _, e := range activity {
		if want[e.Type] {
			n++
		}
	}
	return n
}
