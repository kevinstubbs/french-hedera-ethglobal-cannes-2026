package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/hedera"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// API serves pipeline HTTP handlers.
type API struct {
	Svc *pipeline.Service
	// Hedera optional: top-up verification (deposit path).
	HederaClient hedera.Client
	HederaCfg    config.Hedera
}

type createBody struct {
	AgentID string `json:"agentId"`
}

type streamBody struct {
	Active bool `json:"active"`
}

type reconfigureBody struct {
	Patch map[string]any `json:"patch"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

// Health handles GET /healthz
func (a *API) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// CreatePipeline handles POST /v1/pipelines
func (a *API) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	var body createBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	_ = r.Body.Close()
	if u := config.PrepaidDevAutoCreditUnits(); u > 0 && body.AgentID != "" {
		_ = a.Svc.CreditTopUp(r.Context(), pipeline.AgentTopUpArgs{
			AgentID: body.AgentID, AmountUnits: u, Source: "dev_auto", SourceTxID: "",
		})
	}
	sess, err := a.Svc.Create(r.Context(), body.AgentID)
	if err != nil {
		if errors.Is(err, pipeline.ErrInsufficientPrepaid) {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    sess.ID,
		"state": string(sess.State),
	})
}

// GetStatus handles GET /v1/pipelines/{id}/status
func (a *API) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := a.Svc.Status(id)
	if err != nil {
		if errors.Is(err, pipeline.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := a.Svc.CheckStatusPrepaid(sess); err != nil {
		if errors.Is(err, pipeline.ErrInsufficientPrepaid) {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{
		"id":                   sess.ID,
		"agentId":              sess.AgentID,
		"state":                string(sess.State),
		"billedSeconds":        sess.BilledSeconds,
		"paymentStreamActive":  sess.PaymentStreamActive,
		"rateCentsPerSecond":   sess.RateCentsPerSecond,
		"lastNaryoOpId":        sess.LastNaryoOpID,
		"config":               map[string]any{},
		"rateUnitsPerMinute":   sess.RateUnitsPerMinute,
		"chargedUnits":         sess.ChargedUnits,
		"summaryWindowMinutes": sess.SummaryWindowMinutes,
		"autoPausedForFunds":   sess.AutoPausedForFunds,
	}
	if len(sess.Config) > 0 {
		out["config"] = sess.Config
	}
	if sess.AgentID != "" {
		out["prepaidBalanceUnits"] = a.Svc.PrepaidBalance(sess.AgentID)
	}
	if ev, err := a.Svc.NaryoEventsForSession(id, 10); err == nil {
		out["recentNaryoEvents"] = ev
	}
	writeJSON(w, http.StatusOK, out)
}

type naryoEventBody struct {
	SessionID string         `json:"sessionId"`
	EventID   string         `json:"eventId"`
	Payload   map[string]any `json:"payload"`
}

// PostNaryoEvent handles POST /internal/naryo/v1/events (Naryo broadcaster → platform).
func (a *API) PostNaryoEvent(w http.ResponseWriter, r *http.Request) {
	secret := config.NaryoIngestSecret()
	if secret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "naryo ingest not configured"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Naryo-Webhook-Secret")), []byte(secret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body naryoEventBody
	if !readJSON(w, r, &body) {
		return
	}
	if body.SessionID == "" || body.EventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId and eventId required"})
		return
	}
	dup, err := a.Svc.IngestNaryoEvent(r.Context(), body.SessionID, body.EventID, body.Payload)
	if err != nil {
		if errors.Is(err, pipeline.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		if errors.Is(err, pipeline.ErrInvalidNaryoEvent) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "duplicate": dup})
}

// Start handles POST /v1/pipelines/{id}/start
func (a *API) Start(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Svc.Start(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Stop handles POST /v1/pipelines/{id}/stop
func (a *API) Stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Svc.Stop(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Pause handles POST /v1/pipelines/{id}/pause
func (a *API) Pause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Svc.Pause(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Resume handles POST /v1/pipelines/{id}/resume
func (a *API) Resume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Svc.Resume(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Reconfigure handles PUT /v1/pipelines/{id}/reconfigure
func (a *API) Reconfigure(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body reconfigureBody
	if !readJSON(w, r, &body) {
		return
	}
	if body.Patch == nil {
		body.Patch = map[string]any{}
	}
	if err := a.Svc.Reconfigure(r.Context(), id, body.Patch); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// PaymentStream handles PUT /v1/pipelines/{id}/payment-stream
func (a *API) PaymentStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body streamBody
	if !readJSON(w, r, &body) {
		return
	}
	if err := a.Svc.SetPaymentStreamActive(r.Context(), id, body.Active); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": body.Active})
}

type topUpX402Body struct {
	AmountUnits    int64  `json:"amountUnits"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// TopUpX402 handles POST /v1/agents/{agentId}/topup/x402 (payment-gated). Credits prepaid after x402 succeeds.
func (a *API) TopUpX402(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentId")
	var body topUpX402Body
	if !readJSON(w, r, &body) {
		return
	}
	if agentID == "" || body.AmountUnits <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agentId or amountUnits"})
		return
	}
	ref := body.IdempotencyKey
	if ref == "" {
		ref = r.Header.Get("X-Idempotency-Key")
	}
	err := a.Svc.CreditTopUp(r.Context(), pipeline.AgentTopUpArgs{
		AgentID: agentID, AmountUnits: body.AmountUnits, Source: "x402", SourceTxID: ref,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agentId": agentID, "creditedUnits": body.AmountUnits, "source": "x402",
	})
}

type topUpDepositBody struct {
	TransactionID string `json:"transactionId"`
	AmountUnits   int64  `json:"amountUnits,omitempty"`
}

// TopUpDeposit handles POST /v1/agents/{agentId}/topup/deposit — verifies Hedera transfer then credits.
func (a *API) TopUpDeposit(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentId")
	var body topUpDepositBody
	if !readJSON(w, r, &body) {
		return
	}
	if agentID == "" || body.TransactionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agentId or transactionId"})
		return
	}
	var amount int64
	var asset string
	var err error
	if a.HederaCfg.SkipTopupVerify {
		amount = body.AmountUnits
		if amount <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amountUnits required when HEDERA_SKIP_TOPUP_VERIFY is set"})
			return
		}
		asset = "unspecified"
	} else if a.HederaClient != nil {
		amount, asset, err = a.HederaClient.VerifyTopupTx(
			r.Context(),
			body.TransactionID,
			a.HederaCfg.ServiceAccountID,
			agentID,
		)
		if err != nil {
			slog.Warn("topup deposit verify failed", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	} else {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hedera client not configured"})
		return
	}

	err = a.Svc.CreditTopUp(r.Context(), pipeline.AgentTopUpArgs{
		AgentID: agentID, AmountUnits: amount, Source: "hedera_deposit", SourceTxID: body.TransactionID, Asset: asset,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agentId": agentID, "creditedUnits": amount, "asset": asset, "transactionId": body.TransactionID,
	})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, pipeline.ErrInvalidTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, pipeline.ErrInsufficientPrepaid):
		var prepaid *pipeline.InsufficientPrepaidError
		if errors.As(err, &prepaid) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":                 prepaid.Error(),
				"requiredPrepaidUnits":  prepaid.RequiredUnits,
				"remainingPrepaidUnits": prepaid.RemainingUnits,
			})
		} else {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}
