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
	return m
}
