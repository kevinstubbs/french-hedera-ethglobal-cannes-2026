package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// ObservabilityDeps are wired in main (not payment-gated).
type ObservabilityDeps struct {
	Svc   *pipeline.Service
	Naryo naryo.Client
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
			"pipelines": pipelineSessionMapsForObservability(sessions),
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
			"session":           sessionMapWithNaryoFilterPlan(sess),
			"recentActivity":    deps.Svc.ActivityForSession(id, 50),
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
			"note": "Only POST /v1/pipelines/{id}/start uses x402 (PAYMENT-SIGNATURE); other control-plane mutations use prepaid metering.",
		},
		"prepaid": map[string]any{
			"note":                "Off-chain prepaid units per agent; top up via POST /v1/agents/{agentId}/topup/x402 or .../deposit.",
			"balanceUnitsByAgent": prepaidByAgent,
		},
		"runningPipelines":     runningPaid,
		"streamsActive":        streamOn,
		"estimatedBilledCents": estCents,
		"recentPaymentEvents":  countTypes(activity, []string{"payment_stream", "pipeline_created", "pipeline_started", "agent_top_up"}),
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

// ObservabilityNaryoConfiguration serves GET /observability/v1/naryo/configuration
// (live read from Naryo’s Configuration API: filters, broadcasters, broadcaster-configurations).
// Optional query: pipelineId={sessionId} narrows filters whose names start with pf-{sessionId}- and
// broadcasters whose HTTP destination path contains that session id.
func ObservabilityNaryoConfiguration(deps *ObservabilityDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Naryo == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "naryo client not configured"})
			return
		}
		ctx := r.Context()
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
		}
		snap, err := deps.Naryo.ConfigurationSnapshot(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if pid := strings.TrimSpace(r.URL.Query().Get("pipelineId")); pid != "" {
			snap = narrowNaryoSnapshotForPipeline(snap, pid)
		}
		snap["generatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}

func narrowNaryoSnapshotForPipeline(s map[string]any, pipelineID string) map[string]any {
	prefix := "pf-" + pipelineID + "-"
	destNeedle := "/" + url.PathEscape(pipelineID)
	out := make(map[string]any, len(s)+4)
	for k, v := range s {
		switch k {
		case "orchestratorSessionsCount":
			// Recomputed when orchestratorSessions is narrowed.
			continue
		case "filters":
			out[k] = filterSnapshotFilters(v, prefix)
		case "broadcasters":
			out[k] = filterSnapshotBroadcasters(v, destNeedle)
		case "orchestratorSessions":
			f := filterOrchestratorSessionsForPipeline(v, pipelineID)
			out[k] = f
			out["orchestratorSessionsCount"] = len(f)
		default:
			out[k] = v
		}
	}
	out["pipelineId"] = pipelineID
	out["filterNamePrefix"] = prefix
	out["note"] = "Narrowed filters by name prefix; broadcasters by destination path containing encoded session segment; orchestrator sessions to this pipeline id only."
	return out
}

func filterOrchestratorSessionsForPipeline(v any, sessionID string) []any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []any
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		sid, _ := m["sessionId"].(string)
		if sid == sessionID {
			out = append(out, m)
		}
	}
	return out
}

func filterSnapshotFilters(v any, namePrefix string) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	var out []any
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		n, _ := m["name"].(string)
		if strings.HasPrefix(n, namePrefix) {
			out = append(out, m)
		}
	}
	return out
}

func filterSnapshotBroadcasters(v any, destNeedle string) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	var out []any
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if broadcasterDestinationsContain(m, destNeedle) {
			out = append(out, m)
		}
	}
	return out
}

func broadcasterDestinationsContain(row map[string]any, needle string) bool {
	tgt, _ := row["target"].(map[string]any)
	if tgt == nil {
		return false
	}
	dests, _ := tgt["destinations"].([]any)
	for _, d := range dests {
		s := destinationStringForObs(d)
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func destinationStringForObs(d any) string {
	switch x := d.(type) {
	case string:
		return x
	case map[string]any:
		if v, ok := x["value"].(string); ok {
			return v
		}
		return fmt.Sprint(x)
	default:
		return fmt.Sprint(d)
	}
}

func sessionMapWithNaryoFilterPlan(s pipeline.Session) map[string]any {
	b, err := json.Marshal(s)
	if err != nil {
		return map[string]any{
			"id":              s.ID,
			"naryoFilterPlan": naryo.DescribePipelineFilterPlanForSession(s.ID, s.Config),
		}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{
			"id":              s.ID,
			"naryoFilterPlan": naryo.DescribePipelineFilterPlanForSession(s.ID, s.Config),
		}
	}
	m["naryoFilterPlan"] = naryo.DescribePipelineFilterPlanForSession(s.ID, s.Config)
	return m
}

func pipelineSessionMapsForObservability(sessions []pipeline.Session) []map[string]any {
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionMapWithNaryoFilterPlan(s))
	}
	return out
}
