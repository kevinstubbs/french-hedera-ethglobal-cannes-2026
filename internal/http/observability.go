package httpapi

import (
	"encoding/json"
	"errors"
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

		payments := summarizePayments(deps.Svc, sessions, activity)

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

// ObservabilityPipelineDetail serves GET /observability/v1/pipelines/{id} (session, activity, Naryo events).
func ObservabilityPipelineDetail(deps *ObservabilityDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id"})
			return
		}
		sess, err := deps.Svc.Status(id)
		if err != nil {
			if errors.Is(err, pipeline.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		payload := map[string]any{
			"session":         sess,
			"recentActivity":  deps.Svc.ActivityForSession(id, 50),
			"recentNaryoEvents": []any{},
		}
		if sess.AgentID != "" {
			payload["prepaidBalanceUnits"] = deps.Svc.PrepaidBalance(sess.AgentID)
		}
		if ev, err := deps.Svc.NaryoEventsForSession(id, 25); err == nil && len(ev) > 0 {
			payload["recentNaryoEvents"] = ev
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func summarizePayments(svc *pipeline.Service, sessions []pipeline.Session, activity []pipeline.ActivityEntry) map[string]any {
	var runningPaid, streamOn int
	var estCents int64
	prepaidByAgent := map[string]int64{}
	for _, s := range sessions {
		if s.State == pipeline.StateRunning {
			runningPaid++
			if s.PaymentStreamActive {
				streamOn++
			}
			estCents += s.BilledSeconds * s.RateCentsPerSecond
		}
		if s.AgentID != "" && svc != nil {
			prepaidByAgent[s.AgentID] = svc.PrepaidBalance(s.AgentID)
		}
	}
	return map[string]any{
		"x402": map[string]any{
			"note": "Paid mutations use Coinbase x402 (PAYMENT-SIGNATURE). This summary infers state from sessions + activity only.",
		},
		"prepaid": map[string]any{
			"note":              "Off-chain prepaid units per agent; top up via POST /v1/agents/{agentId}/topup/x402 or .../deposit.",
			"balanceUnitsByAgent": prepaidByAgent,
		},
		"runningPipelines":       runningPaid,
		"streamsActive":          streamOn,
		"estimatedBilledCents":   estCents,
		"recentPaymentEvents": countTypes(activity, []string{"payment_stream", "pipeline_created", "pipeline_started", "agent_top_up"}),
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
