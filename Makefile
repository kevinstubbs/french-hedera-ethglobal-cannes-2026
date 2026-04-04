.PHONY: test test-fast test-full e2e-pipeline

# Default: fast lane (skips Go tests that spawn Node / long e2e when using -short).
test: test-fast

test-fast:
	@echo ""
	@echo "=== Go tests (./cmd/... ./internal/...), -short ==="
	go test ./cmd/... ./internal/... -count=1 -short
	@echo ">>> Go: PASS"
	@echo ""
	@echo "=== agents/trading-signal (Vitest) ==="
	cd agents/trading-signal && npm run test -- --run
	@echo ">>> Vitest: PASS"
	@echo ""
	@echo "All tests passed (fast lane)."

test-full:
	@echo ""
	@echo "=== Go tests (./cmd/... ./internal/...), full ==="
	go test ./cmd/... ./internal/... -count=1
	@echo ">>> Go: PASS"
	@echo ""
	@echo "=== agents/trading-signal (Vitest) ==="
	cd agents/trading-signal && npm run test -- --run
	@echo ">>> Vitest: PASS"
	@echo ""
	@echo "All tests passed (full lane)."

# Pipeline e2e only (mock x402; no live Hedera).
e2e-pipeline:
	@./scripts/e2e-pipeline.sh
