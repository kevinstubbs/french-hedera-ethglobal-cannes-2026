# Runbook

Operational notes for running the Go API and the Next.js observability dashboard locally. Update this file as the stack changes.

## Prerequisites

- **Go** (module targets Go 1.24+; use a recent toolchain).
- **Node.js** and **npm** (for the dashboard).

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

### Useful endpoints

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/healthz` | Liveness; not payment-gated. |
| `GET` | `/observability/v1/summary` | JSON snapshot for the dashboard (sessions, activity, Naryo mock stats, payment summary). **Not authenticated**—do not expose on untrusted networks without a proxy or auth. |
| `GET` | `/v1/pipelines/{id}/status` | Session status (not behind x402 in the current wiring). |
| `POST` | `/v1/pipelines` | Create pipeline (x402). |
| `POST` | `/v1/pipelines/{id}/start` | etc. (x402). |

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

Open **http://localhost:3000**. The UI polls the Go API via a Next.js route handler (no browser CORS to the API).

### Environment

Copy `dashboard/.env.example` to `dashboard/.env.local` if needed:

| Variable | Purpose |
|----------|---------|
| `API_BASE_URL` | Base URL of the Go API **without** a trailing slash (default `http://127.0.0.1:8080`). |

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
