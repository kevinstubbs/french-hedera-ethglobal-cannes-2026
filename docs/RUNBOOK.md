# Runbook

Operational notes for running the Go API and the Next.js observability dashboard locally. Update this file as the stack changes.

## Prerequisites

- **Go** (module targets Go 1.24+; use a recent toolchain).
- **Node.js** and **npm** (for the dashboard).
- **Docker** (optional): for standalone Naryo Configuration API checks — see [NARYO_VERIFY.md](./NARYO_VERIFY.md).

## Naryo standalone (Configuration API)

```bash
cd deploy/naryo-verify && docker compose up -d
# When GET http://127.0.0.1:6060/api/v1/nodes returns 200:
python3 verify_naryo_api.py
```

The stack runs **Anvil** (Ethereum) in Compose and points **Hedera** at your **Solo** mirror on the host (`HEDERA_MIRROR_URL`, default `http://host.docker.internal:5551`). See [NARYO_VERIFY.md](./NARYO_VERIFY.md) for ports and env vars.

See [NARYO_VERIFY.md](./NARYO_VERIFY.md) for scope (Ethereum baseline `PUT` cycle, broadcaster-config + `ALL` broadcasters; filter `POST` skipped on the tested image; declarative Hedera filter in `application.yml`).

## Go API

From the repository root:

```bash
go run ./cmd/api
```

Default listen address is **`:8080`**. Override with:

```bash
PORT=3001 go run ./cmd/api
```

### Environment (x402 / payments)

| Variable | Purpose |
|----------|---------|
| `PORT` | HTTP listen port (default `8080`). |
| `X402_FACILITATOR_URL` | Coinbase / x402 facilitator base URL (default `https://x402.org/facilitator`). |
| `X402_PAY_TO` | Recipient address for paid routes. |
| `X402_NETWORK` | CAIP-2 network (default `eip155:84532`). |
| `X402_PRICE` | Price string per route (default `$0.001`). |

Paid pipeline routes live under **`/v1/`** and require a valid **`PAYMENT-SIGNATURE`** header per x402.

### Prepaid balance (Hedera-oriented MVP)

The API maintains an **off-chain prepaid unit balance per `agentId`**. **Start** and **resume** require at least **`PREPAID_RATE_UNITS_PER_MINUTE`** (default **60** units) so the next wall-clock minute can be charged.

While a pipeline is **running** with an active payment stream, the server:

1. Increments `billedSeconds` every second (same as before).
2. **Once per wall-clock minute**, debits **`rateUnitsPerMinute`** from that agent’s prepaid balance.
3. Emits **`billing_summary`** on HCS (when configured) every **`HEDERA_SUMMARY_WINDOW_MINUTES`** (5–15, default **10**) for batched audit.
4. If a minute charge fails for insufficient balance: **one grace retry** on the next minute boundary; then the pipeline is **auto-paused** (Naryo egress pause best-effort) and **`pipeline_paused`** is logged with reason `insufficient_balance`.

| Variable | Purpose |
|----------|---------|
| `PREPAID_RATE_UNITS_PER_MINUTE` | Units debited per minute while running (default **60** if unset). |
| `PREPAID_DEV_AUTO_CREDIT_UNITS` | **Local demo only:** credit this many units to the agent on `POST /v1/pipelines` create. |
| `HEDERA_NETWORK` | `testnet` (default), `mainnet`, or `previewnet` for the live SDK client. |
| `HEDERA_ENABLED` | `true` / `1` to force-enable Hedera client wiring (optional; auto if operator + topic set). |
| `HEDERA_OPERATOR_ID` | Operator account id for topic submit + record queries. |
| `HEDERA_OPERATOR_KEY` | Operator private key (DER or hex string). |
| `HEDERA_SERVICE_ACCOUNT_ID` | Expected **recipient** of deposit transfers for `VerifyTopupTx`. |
| `HEDERA_AUDIT_TOPIC_ID` | HCS topic id; envelope JSON is submitted asynchronously (best-effort). |
| `HEDERA_SUMMARY_WINDOW_MINUTES` | Billing summary batch window (clamped **5–15**). |
| `HEDERA_SKIP_TOPUP_VERIFY` | If `true`, `.../topup/deposit` trusts **`amountUnits`** in the body (demo only). |

**Top-up (x402, preferred):** `POST /v1/agents/{agentId}/topup/x402` with body `{"amountUnits":1000,"idempotencyKey":"optional"}`. Payment-gated like other `/v1` mutations.

**Top-up (Hedera deposit):** `POST /v1/agents/{agentId}/topup/deposit` with `{"transactionId":"0.0.x@..."}`. The server loads the transaction record and credits inbound amount to the service account (tinybar / token units). Include `agentId` in the **memo** when sending the transfer if you rely on memo checks.

**Quick local flow without a separate top-up call:**

```bash
PREPAID_DEV_AUTO_CREDIT_UNITS=100000 go run ./cmd/api
```

### Useful endpoints

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/healthz` | Liveness; not payment-gated. |
| `GET` | `/observability/v1/summary` | JSON snapshot for the dashboard (sessions, activity, Naryo mock stats, payment summary). **Not authenticated**—do not expose on untrusted networks without a proxy or auth. |
| `GET` | `/v1/pipelines/{id}/status` | Session status (not behind x402 in the current wiring). |
| `POST` | `/v1/pipelines` | Create pipeline (x402). |
| `POST` | `/v1/pipelines/{id}/start` | etc. (x402). |
| `POST` | `/v1/agents/{agentId}/topup/x402` | Credit prepaid after x402 (x402). |
| `POST` | `/v1/agents/{agentId}/topup/deposit` | Credit prepaid from verified Hedera tx (x402). |

### Tests

Run from the **repository root** (so the module root and all packages are in scope):

```bash
go test ./... -count=1
```

Running `go test ./...` from `internal/` only covers packages under `internal/`; that is fine, but the output pauses after the early packages while Go **compiles `internal/http`** for the first time. That package depends on **x402 → go-ethereum**, so the **first** build can take **several minutes** and looks idle. It is not stuck unless the CPU is actually idle for a long time; wait for the compile to finish or watch build progress with:

```bash
go test -v ./internal/http -count=1
```

Optional safety cap:

```bash
go test ./... -count=1 -timeout 5m
```

## Observability dashboard (Next.js)

```bash
cd dashboard
npm install
npm run dev
```

Open **http://localhost:3000**. The UI polls the Go API via a Next.js route handler (no browser CORS to the API). Use the **Hedera** tab to embed the Solo Explorer UI in an iframe when you run a local network.

### Local Hedera with Solo

Deploy a local Hiero/Hedera stack (Explorer UI on **http://localhost:8080** by default) as described in the [Solo user guide](https://solo.hiero.org/v0.60.0/docs/solo-user-guide/):

```bash
solo one-shot single deploy
```

Teardown when finished:

```bash
solo one-shot single destroy
```

### Environment

Copy `dashboard/.env.example` to `dashboard/.env.local` if needed:

| Variable | Purpose |
|----------|---------|
| `API_BASE_URL` | Base URL of the Go API **without** a trailing slash (default `http://127.0.0.1:8080`). |
| `NEXT_PUBLIC_HEDERA_EXPLORER_URL` | URL of the Explorer UI for the **Hedera** tab iframe (default `http://localhost:8080`). |

Production build:

```bash
cd dashboard
npm run build
npm start
```

## Typical two-terminal workflow

**Terminal 1 — API**

```bash
go run ./cmd/api
```

**Terminal 2 — dashboard**

```bash
cd dashboard && npm run dev
```

If the API is not on `127.0.0.1:8080`, set `API_BASE_URL` in `dashboard/.env.local`.
