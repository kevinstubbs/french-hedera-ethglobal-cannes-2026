package httpapi

import (
	"net/http"
)

// NewMux registers all HTTP routes on a ServeMux (Go 1.22+ patterns).
func NewMux(api *API) *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", api.Health)
	m.HandleFunc("GET /v1/pipelines/{id}/status", api.GetStatus)
	m.HandleFunc("POST /v1/pipelines", api.CreatePipeline)
	m.HandleFunc("POST /v1/pipelines/{id}/start", api.Start)
	m.HandleFunc("POST /v1/pipelines/{id}/stop", api.Stop)
	m.HandleFunc("POST /v1/pipelines/{id}/pause", api.Pause)
	m.HandleFunc("POST /v1/pipelines/{id}/resume", api.Resume)
	m.HandleFunc("PUT /v1/pipelines/{id}/reconfigure", api.Reconfigure)
	m.HandleFunc("PUT /v1/pipelines/{id}/payment-stream", api.PaymentStream)
	m.HandleFunc("POST /v1/agents/{agentId}/topup/x402", api.TopUpX402)
	m.HandleFunc("POST /v1/agents/{agentId}/topup/deposit", api.TopUpDeposit)
	return m
}

// RegisterInternalRoutes attaches internal routes that are not behind x402 (e.g. Naryo webhooks).
func RegisterInternalRoutes(m *http.ServeMux, api *API) {
	// Session in path: Naryo HTTP broadcaster destination per pipeline (native JSON body).
	m.HandleFunc("POST /internal/naryo/v1/events/{sessionId}", api.PostNaryoEventSession)
	m.HandleFunc("POST /internal/naryo/v1/events", api.PostNaryoEvent)
}
