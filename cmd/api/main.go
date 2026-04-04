package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	x402http "github.com/coinbase/x402/go/http"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/billing"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
	hcslog "github.com/french-hedera-ethglobal-cannes2026/submission/internal/hcs"
	httpapi "github.com/french-hedera-ethglobal-cannes2026/submission/internal/http"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/http/middleware"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/hedera"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/ledger"
	pipelinemcp "github.com/french-hedera-ethglobal-cannes2026/submission/internal/mcp"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	xcfg := config.LoadX402FromEnv()
	hedCfg := config.LoadHederaFromEnv()
	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: xcfg.FacilitatorURL,
	})

	store := pipeline.NewMemoryStore()
	naryoClient := &naryo.MockClient{}
	hedCli := hedera.NewClientFromConfig(hedCfg)

	var hcsOpts []hcslog.LoggerOption
	if hedCfg.AuditTopicID != "" && hedCfg.OperatorAccountID != "" && hedCfg.OperatorPrivateKey != "" {
		hcsOpts = append(hcsOpts, hcslog.WithHCSTopic(hedCli, hedCfg.AuditTopicID))
	}
	hcs := hcslog.NewLogger(hcsOpts...)
	activity := pipeline.NewActivityLog(256)

	led := ledger.NewMemoryLedger()
	svcOpts := []pipeline.ServiceOption{
		pipeline.WithPrepaidLedger(led),
		pipeline.WithSummaryWindowMinutes(hedCfg.SummaryWindowMinutes),
	}
	if ru := config.PrepaidRateUnitsPerMinute(); ru > 0 {
		svcOpts = append(svcOpts, pipeline.WithRateUnitsPerMinute(ru))
	}
	svc := pipeline.NewService(store, naryoClient, hcs, 1, activity, svcOpts...)

	billingCtx, billingCancel := context.WithCancel(ctx)
	defer billingCancel()
	go billing.RunEach(billingCtx, time.Second, func() {
		svc.BillingTick(billingCtx)
	})

	api := &httpapi.API{Svc: svc, HederaClient: hedCli, HederaCfg: hedCfg}
	mux := httpapi.NewMux(api)
	guarded := middleware.PaymentGate(xcfg, facilitator, true)(mux)

	obs := &httpapi.ObservabilityDeps{Svc: svc, Naryo: naryoClient}

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.Health)
	root.HandleFunc("GET /observability/v1/summary", httpapi.ObservabilitySummary(obs))
	root.Handle("/v1/", guarded)
	root.Handle("/mcp", pipelinemcp.StreamableHTTPHandler(svc))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", addr, "mcp_path", "/mcp", "hedera_audit_topic", hedCfg.AuditTopicID != "")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
