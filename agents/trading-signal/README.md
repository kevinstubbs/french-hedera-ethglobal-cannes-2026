# Trading signal agent (scaffold)

TypeScript package for the demo **trading / signal** agent: **x402** against the Go control plane (`PipelineRoutes`) and **Hedera Agent Kit** for **HBAR** actions (poll loop and transfers are follow-up work).

## Setup

```bash
cd agents/trading-signal
npm install
npm run typecheck
npm run build   # emits dist/ (gitignored); required before importing the published package main path
```

Copy `.env.example` to `.env` and fill in keys. Operational context: [docs/RUNBOOK.md](../../docs/RUNBOOK.md), scope: [docs/PROJECT_SCOPE.md](../../docs/PROJECT_SCOPE.md).

## x402-aware fetch

```typescript
import { createControlPlaneFetch, pipelineRouteRequiresX402 } from "trading-signal";

const fetchPaid = createControlPlaneFetch({
  evmPrivateKey: process.env.X402_EVM_PRIVATE_KEY as `0x${string}`,
  x402Network: process.env.X402_NETWORK ?? "eip155:84532",
  evmRpcUrl: process.env.EVM_RPC_URL,
});

await fetchPaid(`${process.env.API_BASE_URL}/v1/pipelines`, { method: "POST", body: "{}", headers: { "Content-Type": "application/json" } });
```

`pipelineRouteRequiresX402(method, pathname)` mirrors `internal/http/middleware/x402.go` so you can assert or branch before calling paid endpoints.
