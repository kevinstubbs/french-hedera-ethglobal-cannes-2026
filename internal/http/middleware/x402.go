package middleware

import (
	"net/http"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	x402net "github.com/coinbase/x402/go/http/nethttp"
	evmserver "github.com/coinbase/x402/go/mechanisms/evm/exact/server"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// PipelineRoutes builds x402 route keys for all payment-gated pipeline controls.
func PipelineRoutes(cfg config.X402) x402http.RoutesConfig {
	rc := x402http.RouteConfig{
		Accepts: x402http.PaymentOptions{
			{
				Scheme:  "exact",
				PayTo:   cfg.PayTo,
				Price:   cfg.Price,
				Network: x402.Network(cfg.Network),
			},
		},
		Description: "Pipeline control",
		MimeType:    "application/json",
	}
	return x402http.RoutesConfig{
		"POST /v1/pipelines":                      rc,
		"POST /v1/pipelines/*/start":              rc,
		"POST /v1/pipelines/*/stop":               rc,
		"POST /v1/pipelines/*/pause":              rc,
		"POST /v1/pipelines/*/resume":             rc,
		"PUT /v1/pipelines/*/reconfigure":         rc,
		"PUT /v1/pipelines/*/payment-stream":      rc,
		"POST /v1/agents/*/topup/x402":            rc,
		"POST /v1/agents/*/topup/deposit":         rc,
	}
}

// PaymentGate returns middleware that enforces x402 on routes registered in PipelineRoutes.
// facilitator is typically x402http.NewHTTPFacilitatorClient; tests may pass a mock implementation.
func PaymentGate(cfg config.X402, facilitator x402.FacilitatorClient, syncFacilitatorOnStart bool) func(http.Handler) http.Handler {
	routes := PipelineRoutes(cfg)
	return x402net.PaymentMiddlewareFromConfig(
		routes,
		x402net.WithFacilitatorClient(facilitator),
		x402net.WithScheme(x402.Network(cfg.Network), evmserver.NewExactEvmScheme()),
		x402net.WithSyncFacilitatorOnStart(syncFacilitatorOnStart),
	)
}
