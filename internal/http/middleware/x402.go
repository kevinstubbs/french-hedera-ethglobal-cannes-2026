package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	x402net "github.com/coinbase/x402/go/http/nethttp"
	evmserver "github.com/coinbase/x402/go/mechanisms/evm/exact/server"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// PipelineRoutes builds x402 route keys for payment-gated control-plane calls.
// Only POST /v1/pipelines/{id}/start is gated; the on-chain amount is X402_PRICE × X402_START_RUNWAY_SECONDS.
// GET /v1/pipelines/{id}/status is not x402-gated — it requires prepaid balance > 0 (running/paused + ledger) in the handler.
// Prepaid top-up (POST /v1/agents/{id}/topup/x402) is not gated here.
func PipelineRoutes(cfg config.X402) x402http.RoutesConfig {
	runSecs := cfg.StartRunwaySeconds
	if runSecs <= 0 {
		runSecs = 300
	}
	dyn := x402http.DynamicPriceFunc(func(ctx context.Context, reqCtx x402http.HTTPRequestContext) (x402.Price, error) {
		_ = ctx
		_ = reqCtx
		return config.MulUSDPrice(cfg.Price, runSecs)
	})
	rc := x402http.RouteConfig{
		Accepts: x402http.PaymentOptions{
			{
				Scheme:  "exact",
				PayTo:   cfg.PayTo,
				Price:   dyn,
				Network: x402.Network(cfg.Network),
			},
		},
		Description: "Pipeline start",
		MimeType:    "application/json",
	}
	return x402http.RoutesConfig{
		"POST /v1/pipelines/*/start": rc,
	}
}

// PaymentGate returns middleware that enforces x402 on routes registered in PipelineRoutes.
// facilitator is typically x402http.NewHTTPFacilitatorClient; tests may pass a mock implementation.
func PaymentGate(cfg config.X402, facilitator x402.FacilitatorClient, syncFacilitatorOnStart bool) func(http.Handler) http.Handler {
	routes := PipelineRoutes(cfg)
	serverOpts := []x402.ResourceServerOption{x402.WithFacilitatorClient(facilitator)}
	httpServer := x402http.Newx402HTTPResourceServer(routes, serverOpts...)
	httpServer.Register(x402.Network(cfg.Network), evmserver.NewExactEvmScheme())
	timeout := 30 * time.Second
	if syncFacilitatorOnStart {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := httpServer.Initialize(ctx); err != nil {
			fmt.Printf("Warning: failed to initialize x402 server: %v\n", err)
		}
	}
	return x402net.PaymentMiddlewareFromHTTPServer(httpServer,
		x402net.WithFacilitatorClient(facilitator),
		x402net.WithSyncFacilitatorOnStart(false),
		x402net.WithTimeout(timeout),
	)
}
