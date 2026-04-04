#!/usr/bin/env bash
# Automated pipeline e2e: x402 (mock facilitator auto-pay), optional prepaid ledger path,
# simulated Naryo ingest, status with recentNaryoEvents.
# Requires: Go toolchain, repo root as cwd.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
exec go test ./internal/http \
	-run 'TestE2E_(CreateStartNaryoIngestStatus|FullPrepaidAndX402Flow)$' \
	-count=1 -v
