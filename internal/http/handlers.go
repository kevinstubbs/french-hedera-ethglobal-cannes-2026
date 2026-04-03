package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// API serves pipeline HTTP handlers.
type API struct {
	Svc *pipeline.Service
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
	sess, err := a.Svc.Create(r.Context(), body.AgentID)
	if err != nil {
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
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  sess.ID,
		"agentId":             sess.AgentID,
		"state":               string(sess.State),
		"billedSeconds":       sess.BilledSeconds,
		"paymentStreamActive": sess.PaymentStreamActive,
		"rateCentsPerSecond":  sess.RateCentsPerSecond,
		"lastNaryoOpId":       sess.LastNaryoOpID,
	})
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

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, pipeline.ErrInvalidTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}
