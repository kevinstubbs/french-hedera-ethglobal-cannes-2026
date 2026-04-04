# Pipeline event routing (Naryo → platform API → agents)

This describes the **intended production shape** for how indexed chain data reaches agents. It is separate from the standalone stack in [`NARYO_VERIFY.md`](./NARYO_VERIFY.md), which exercises Naryo’s Configuration API in isolation (e.g. Mockoon as a dummy HTTP target).

## Principles

1. **Naryo never calls agent-owned URLs directly.** All HTTP broadcasters in Naryo point at **this platform’s API** (one or a few stable internal URLs). This repo implements **`POST /internal/naryo/v1/events`** (not x402-gated; authenticate with **`X-Naryo-Webhook-Secret`** matching env **`NARYO_INGEST_SECRET`**).

2. **The API is the control plane for delivery.** When a payload arrives from Naryo, the API:
   - authenticates the caller (shared secret, mTLS, or network policy — not agent API keys on Naryo),
   - resolves which **pipeline / session / agent** the event belongs to (via Naryo metadata, correlation ids, or mapping you maintain when provisioning Naryo),
   - **persists** the normalized event (durable log / DB) for replay and audits,
   - **optionally forwards** to the agent’s **webhook** if that pipeline’s settings include one,
   - applies **backpressure / retries** for agent delivery without touching Naryo.

3. **Pull is first-class.** Agents that do not configure a webhook (or that miss deliveries) call **your API** for history, e.g. “events for this subscription / pipeline since cursor or since time **T**.” Naryo remains the indexer; the platform API is the **subscription and delivery** boundary agents see.

## Why not point Naryo at the agent’s webhook?

- **Security:** Agent URLs would need to be registered inside Naryo; rotation, SSRF, and abuse surface grow quickly.
- **Policy:** Billing, pause/resume, and “who may receive what” stay in one place (the Go API + pipeline session state).
- **Reliability:** You can queue, dedupe, and retry agent delivery without Naryo’s broadcaster semantics being your only line of defense.

## Implementation notes (when you wire a real Naryo client)

- **Provisioning:** When `EnsurePipeline` (or equivalent) runs, create/update Naryo **broadcaster-configuration** entries whose `endpoint.url` is **only** platform-internal endpoints; per-pipeline agent webhooks live in **session / agent config** in the API, not in Naryo.
- **Correlation:** Define a stable id (pipeline session id, Naryo filter/broadcaster id, or custom header) so inbound Naryo POSTs can be tied to `Session` / `AgentID` in [`internal/pipeline`](../internal/pipeline/).
- **Observability:** Today [`internal/naryo`](../internal/naryo/) is a mock; extend `Client` and observability once real Configuration API calls define broadcaster targets.

## Related

- Pipeline lifecycle and Naryo egress pause: [`RUNBOOK.md`](./RUNBOOK.md) (prepaid / pause behavior).
- Local Naryo API smoke test: [`NARYO_VERIFY.md`](./NARYO_VERIFY.md).
