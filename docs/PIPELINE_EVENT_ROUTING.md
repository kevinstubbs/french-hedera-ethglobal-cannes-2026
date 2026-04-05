# Pipeline event routing (Naryo → platform API → agents)

This describes the **intended production shape** for how indexed chain data reaches agents. It is separate from the standalone stack in [`NARYO_VERIFY.md`](./NARYO_VERIFY.md), which exercises Naryo’s Configuration API in isolation (e.g. Mockoon as a dummy HTTP target).

## Principles

1. **Naryo never calls agent-owned URLs directly.** All HTTP broadcasters in Naryo point at **this platform’s API**. Production ingest is **`POST /internal/naryo/v1/events/{sessionId}`** with Naryo’s native JSON body; the path segment is the pipeline session id so you do not depend on Naryo embedding that id in the payload. **`POST /internal/naryo/v1/events`** with `sessionId` in JSON remains for tests and manual tools. Not x402-gated; authenticate with **`X-Naryo-Webhook-Secret`** = **`NARYO_INGEST_SECRET`**.

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

## Implementation notes (real Naryo client)

- **Provisioning:** On **start / resume**, `EnsurePipeline` reads **`session.Config`** (including `eventSubscriptions` from `PUT .../reconfigure`). It **`POST /api/v1/filters`** once per pipeline (name `pf-{sessionId}-hcs` or `-evm`), then **`POST /api/v1/broadcasters`** with target **`FILTER`** and that filter id. HTTP `destinations` are **`{NARYO_BROADCASTER_DEST_PATH}/{sessionId}`** (default `/internal/naryo/v1/events/...`). Broadcaster-configuration `endpoint.url` is **`NARYO_PLATFORM_INGEST_URL`** only. **Node UUIDs** in filter bodies must match Naryo’s **`application.yml`** (`NARYO_HEDERA_NODE_ID`, `NARYO_ETHEREUM_NODE_ID`; defaults match `deploy/naryo-verify/application.yml`).
- **Subscription selection:** First **`hedera_hcs_topic`** subscription wins (Hedera `TRANSACTION` filter, `IDENTITY_ID` = topic id). Otherwise first **`erc20_transfer`** → **`EVENT_CONTRACT`** on that token for **`Transfer(address,address,uint256)`** (all such transfers on the contract; a specific `fromAddress` in the patch is **not** applied in Naryo today). **`NARYO_DEFAULT_HCS_TOPIC_ID`** applies when there is no Hedera entry in `eventSubscriptions`. Only when **no** scoped plan exists, if **`NARYO_ALLOW_ALL_BROADCASTER_FALLBACK`** is true (default), the client provisions a broadcaster with target **`ALL`** (legacy). If it is false, **`EnsurePipeline`** errors until you add subscriptions or the default topic env.
- **Correlation:** The URL path carries the pipeline session id; dedupe uses `eventId` from common Naryo id fields or a hash-derived fallback.
- **Observability:** The API requires `NARYO_CONFIG_API_BASE` and `NARYO_PLATFORM_INGEST_URL` at startup; observability reports the live HTTP Naryo client stats and configuration snapshot.

## Reconfigure vs Naryo

`PUT .../reconfigure` only updates **`session.Config`** until the next **`EnsurePipeline`**. **Call reconfigure before start**, or **pause → resume** (or stop → start) so Naryo filters/broadcasters are recreated from the new config. Naryo **nodes, mirror URL, and RPC** still come from **Naryo’s own config** (`application.yml` / env); the platform does not add nodes via the API here.

## Related

- Pipeline lifecycle and Naryo egress pause: [`RUNBOOK.md`](./RUNBOOK.md) (prepaid / pause behavior).
- Local Naryo API smoke test: [`NARYO_VERIFY.md`](./NARYO_VERIFY.md).
