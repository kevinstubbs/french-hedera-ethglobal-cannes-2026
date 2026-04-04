# Runbook

Operational notes for running the Go API and the Next.js observability dashboard locally. Update this file as the stack changes.

## Prerequisites

- **Go** (module targets Go 1.24+; use a recent toolchain).
- **Node.js** and **npm** (for the dashboard).
- **Docker** (optional): for standalone Naryo Configuration API checks — see [NARYO_VERIFY.md](./NARYO_VERIFY.md).

## Testing (layered)

From the repository root:


| Command                                     | Purpose                                                                                                      |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `make test-fast`                            | Go unit tests with `-short` (skips tests that spawn Node or full mock e2e) + `agents/trading-signal` Vitest. |
| `make test-full`                            | All Go tests in `./cmd/...` and `./internal/...` (including Node pairwise + ingest e2e) + Vitest.            |
| `go test ./cmd/... ./internal/... -count=1` | Go only (full).                                                                                              |
| `cd agents/trading-signal && npm test`      | Agent package unit tests only.                                                                               |


`make test-full` / `make test-fast` print `**>>> Go: PASS`** and `**>>> Vitest: PASS**` and end with `**All tests passed**` when everything succeeds (any failure stops the recipe with a non-zero exit code).

**Reading raw `go test` lines:** each line is one package. `**ok`** means that package’s tests passed (timing on the right). `**?**` means **no test files** in that package — that is normal, not a failure. Only `**FAIL`** indicates failing tests. On narrow terminals, tabs can make columns look merged (e.g. `cmd/api` touching `[no test files]`); widen the window or rely on the Makefile summary lines above.

CI runs Go (`./cmd/...` `./internal/...`) plus `agents/trading-signal` Vitest — see `[.github/workflows/test.yml](../.github/workflows/test.yml)`. Use `make test-fast` locally for the `-short` Go lane.

**Contract parity:** Go `middleware.PipelineRoutes` must match `agents/trading-signal/src/pipelineX402Routes.ts` — enforced by `TestPipelineRoutesMatchTypeScriptMirror` in `internal/http/middleware`.

## Naryo standalone (Configuration API)

```bash
cd deploy/naryo-verify && docker compose up -d
# When GET http://127.0.0.1:6060/api/v1/nodes returns 200:
python3 verify_naryo_api.py
```

The stack runs **Anvil** (Ethereum) in Compose and points **Hedera** at a **local mirror REST** URL on the host (`HEDERA_MIRROR_URL`, default `http://host.docker.internal:5551`), matching **[hedera-local-node](../hedera-local-node/README.md)** mirror port **5551** when that stack runs on the host. See [NARYO_VERIFY.md](./NARYO_VERIFY.md) for ports and env vars.

See [NARYO_VERIFY.md](./NARYO_VERIFY.md) for scope (Ethereum baseline `PUT` cycle, broadcaster-config + `ALL` broadcasters; filter `POST` skipped on the tested image; declarative Hedera filter in `application.yml`).

**Production intent:** Naryo HTTP broadcasters should target **this platform’s API only**; the API stores events and optionally forwards to per-pipeline agent webhooks, while agents without webhooks use **pull** APIs for history. See [PIPELINE_EVENT_ROUTING.md](./PIPELINE_EVENT_ROUTING.md).

## Go API

From the repository root:

```bash
go run ./cmd/api
```

Default listen address is `**:8080**`. Override with:

```bash
PORT=3001 go run ./cmd/api
```

### Environment (x402 / payments)


| Variable               | Purpose                                                                                                                     |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `PORT`                 | HTTP listen port (default `8080`).                                                                                          |
| `X402_FACILITATOR_URL` | Coinbase / x402 facilitator base URL (default `https://x402.org/facilitator`).                                              |
| `X402_PAY_TO`          | Recipient address for paid routes.                                                                                          |
| `X402_NETWORK`         | CAIP-2 network (default `eip155:84532`).                                                                                    |
| `X402_PRICE`           | Price for **start** only (default `**$0.01`**).                                                                             |
| `NARYO_INGEST_SECRET`  | Shared secret for `POST /internal/naryo/v1/events` (header `**X-Naryo-Webhook-Secret**`). If unset, ingest returns **503**. |


Only `**POST /v1/pipelines/{id}/start`** is x402-gated and requires a valid `**PAYMENT-SIGNATURE**`. Other `/v1` mutations use the prepaid ledger (no x402 on each call).

### Prepaid balance (Hedera-oriented MVP)

The API maintains an **off-chain prepaid unit balance per `agentId`**. **Start** and **resume** require at least `**PREPAID_MIN_START_MINUTES` × `PREPAID_RATE_UNITS_PER_MINUTE`** units (defaults **10** minutes × **1** unit/min, i.e. **10** units). Treat **one unit** as **$0.01** of runway when using the default rate (**1** unit per wall-clock minute per running pipeline).

While a pipeline is **running** with an active payment stream, the server:

1. Increments `**billedSeconds`** every billing tick (the API runs `**BillingTick` once per second**).
2. Accrues usage with **second precision** (`rateUnitsPerMinute` is spread across seconds), then debits the ledger **every `PREPAID_DEBIT_INTERVAL_SECONDS`** (default **300** ≈ five minutes) in one charge.
3. Emits `**billing_summary`** on HCS (when configured) every `**HEDERA_SUMMARY_WINDOW_MINUTES**` (5–15, default **10**) for batched audit.
4. If a debit fails for insufficient balance: **one grace retry** after **60 seconds**; then the pipeline is **auto-paused** (Naryo egress pause best-effort) and `**pipeline_paused`** is logged with reason `insufficient_balance`.


| Variable                         | Purpose                                                                                                                                                                    |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PREPAID_RATE_UNITS_PER_MINUTE`  | Units debited per **minute** of runtime while running (default **1** if unset → **$0.01**/minute when 1 unit = 1¢).                                                        |
| `PREPAID_DEBIT_INTERVAL_SECONDS` | How often accumulated usage is charged (default **300**).                                                                                                                  |
| `PREPAID_MIN_START_MINUTES`      | Minimum prepaid runway in **minutes at the per-minute rate** for **start**/**resume** (default **10**).                                                                    |
| `PREPAID_DEV_AUTO_CREDIT_UNITS`  | **Local demo only:** credit this many units to the agent on `POST /v1/pipelines` create.                                                                                   |
| `HEDERA_NETWORK`                 | `testnet` (default), `mainnet`, `previewnet`, or `**local`** for Hiero SDK `ClientForName("local")` (typical with **hedera-local-node**; see `**cmd/hcs-demo-activity`**). |
| `HEDERA_ENABLED`                 | `true` / `1` to force-enable Hedera client wiring (optional; auto if operator + topic set).                                                                                |
| `HEDERA_OPERATOR_ID`             | Operator account id for topic submit + record queries.                                                                                                                     |
| `HEDERA_OPERATOR_KEY`            | Operator private key (DER or hex string).                                                                                                                                  |


**Local network (hedera-local-node):** use the relay operator from `**hedera-local-node/.env`** — set `HEDERA_OPERATOR_ID` to `**RELAY_OPERATOR_ID_MAIN**` (typically `0.0.2`) and `HEDERA_OPERATOR_KEY` to `**RELAY_OPERATOR_KEY_MAIN**` (DER hex string). Consensus node account `**0.0.3**` in that file is the **node** id for gRPC maps, not the signing operator.
| `HEDERA_SERVICE_ACCOUNT_ID` | Expected **recipient** of deposit transfers for `VerifyTopupTx`. |
| `HEDERA_AUDIT_TOPIC_ID` | HCS topic id; envelope JSON is submitted asynchronously (best-effort). |
| `HEDERA_SUMMARY_WINDOW_MINUTES` | Billing summary batch window (clamped **5–15**). |
| `HEDERA_SKIP_TOPUP_VERIFY` | If `true`, `.../topup/deposit` trusts `**amountUnits`** in the body (demo only). |

**Top-up (x402 body, no HTTP x402 gate):** `POST /v1/agents/{agentId}/topup/x402` with body `{"amountUnits":1000,"idempotencyKey":"optional"}` credits prepaid after the handler runs (not payment-gated at the HTTP layer).

**Top-up (Hedera deposit):** `POST /v1/agents/{agentId}/topup/deposit` with `{"transactionId":"0.0.x@..."}`. The server loads the transaction record and credits inbound amount to the service account (tinybar / token units). Include `agentId` in the **memo** when sending the transfer if you rely on memo checks.

**Quick local flow without a separate top-up call:**

```bash
PREPAID_DEV_AUTO_CREDIT_UNITS=100000 go run ./cmd/api
```

### Useful endpoints


| Method | Path                                 | Notes                                                                                                                                                                       |
| ------ | ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/healthz`                           | Liveness; not payment-gated.                                                                                                                                                |
| `GET`  | `/observability/v1/summary`          | JSON snapshot for the dashboard (sessions, activity, Naryo mock stats, payment summary). **Not authenticated**—do not expose on untrusted networks without a proxy or auth. |
| `GET`  | `/v1/pipelines/{id}/status`          | Session status (not behind x402 in the current wiring).                                                                                                                     |
| `POST` | `/v1/pipelines`                      | Create pipeline (not x402-gated).                                                                                                                                           |
| `POST` | `/v1/pipelines/{id}/start`           | **x402** (PAYMENT-SIGNATURE).                                                                                                                                               |
| `POST` | `/v1/pipelines/{id}/stop`            | etc. — prepaid metering only, no x402.                                                                                                                                      |
| `POST` | `/v1/agents/{agentId}/topup/x402`    | Credit prepaid (not HTTP x402-gated).                                                                                                                                       |
| `POST` | `/v1/agents/{agentId}/topup/deposit` | Credit prepaid from verified Hedera tx (not HTTP x402-gated).                                                                                                               |


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

Open **[http://localhost:3000](http://localhost:3000)**. The UI polls the Go API via a Next.js route handler (no browser CORS to the API). Use the **Hedera** tab to embed the **Hedera Local Node** mirror explorer in an iframe when you run a local network.

### Local Hedera (hedera-local-node)

This repository vendors **[hedera-local-node](../hedera-local-node/README.md)** (Hiero / Hashgraph local consensus + mirror). From that directory:

```bash
cd hedera-local-node
npm install
npm run start
```

Mirror REST (and Naryo’s `HEDERA_MIRROR_URL` toward the host) is typically `**http://127.0.0.1:5551**`. The bundled Explorer UI defaults to `**http://localhost:8090**` (set `NEXT_PUBLIC_HEDERA_EXPLORER_URL` in the dashboard if yours differs).

Stop the network from the same directory (see upstream README for `npm run stop` / CLI options).

### Environment

Copy `dashboard/.env.example` to `dashboard/.env.local` if needed:


| Variable                          | Purpose                                                                                                       |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `API_BASE_URL`                    | Base URL of the Go API **without** a trailing slash (default `http://127.0.0.1:8080`).                        |
| `NEXT_PUBLIC_HEDERA_EXPLORER_URL` | URL of the Explorer UI for the **Hedera** tab iframe (default `http://localhost:8090` for hedera-local-node). |


Production build:

```bash
cd dashboard
npm run build
npm start
```

## Demo and e2e scripts

### Hedera HCS traffic (Naryo mirror demos)

`[cmd/hcs-demo-activity](../cmd/hcs-demo-activity/main.go)` submits JSON messages to an HCS topic on a loop so **Naryo** (Hedera mirror node in `deploy/naryo-verify`) can index **transactions** for that topic when `**LOCAL_NODE_HCS_TOPIC_ID`** (or legacy `SOLO_HCS_TOPIC_ID`) matches the same `0.0.n` topic in compose.

From the repo root (with **hedera-local-node** or public-network credentials and a topic id):

```bash
# Operator + key: copy from hedera-local-node/.env → RELAY_OPERATOR_ID_MAIN / RELAY_OPERATOR_KEY_MAIN
export HEDERA_OPERATOR_ID=0.0.2
export HEDERA_OPERATOR_KEY=302e020100300506032b657004220420...
export HEDERA_NETWORK=local   # or testnet / mainnet / previewnet
export HEDERA_LOCAL_PLAINTEXT=1   # typical for local-node gRPC (plaintext to consensus node)
go run ./cmd/hcs-demo-activity -topic 0.0.y -interval 5s
```

One-shot:

```bash
go run ./cmd/hcs-demo-activity -once -topic 0.0.y
```

The topic id must **already exist** on that ledger (arbitrary ids like `0.0.123` fail with `INVALID_TOPIC_ID` until created). Create one, then reuse its id:

```bash
cd cmd/hcs-demo-activity
go run . -create-topic
go run . -topic 0.0.<printed> -interval 5s
```

Topic resolution: `-topic`, then `HEDERA_HCS_TOPIC_ID`, `LOCAL_NODE_HCS_TOPIC_ID`, `SOLO_HCS_TOPIC_ID` (deprecated alias), `HEDERA_AUDIT_TOPIC_ID`. Override local node addresses with `HEDERA_LOCAL_GRPC`, `HEDERA_LOCAL_NODE_ACCOUNT_ID` (often `**0.0.3**` for hedera-local-node’s consensus node), `HEDERA_LOCAL_MIRROR` (comma-separated).

The command loads `**.env**` from the current directory (and `**cmd/hcs-demo-activity/.env**` when you run from the repo root). It also accepts `**RELAY_OPERATOR_ID_MAIN` / `RELAY_OPERATOR_KEY_MAIN**` if you copied `**hedera-local-node/.env**`. If you use `**source .env**` in bash, use `**export VAR=...**` lines or `**set -a && source .env && set +a**`, or `go run` will not inherit unexported variables.

### Pipeline e2e (x402 + ingest)

`[scripts/e2e-pipeline.sh](../scripts/e2e-pipeline.sh)` runs Go tests that spin up the HTTP stack with a **mock x402 facilitator** (auto-pay), create/start pipeline, POST a synthetic Naryo webhook, and assert `recentNaryoEvents` on status (including a **prepaid ledger + top-up + start** variant).

```bash
./scripts/e2e-pipeline.sh
```

Same tests run under `make test-full` (no `-short`) as part of `./internal/http`.

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