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
