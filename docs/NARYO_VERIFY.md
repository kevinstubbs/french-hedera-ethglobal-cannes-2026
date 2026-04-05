# Naryo Configuration API (standalone verification)

This proves **CRUD** plus **`GET /api/v1/operations/{id}`** polling for:

- **Broadcaster configurations** — create, update, delete.
- **Broadcasters** — create, update, delete with target type **`ALL`** (see below).
- **Nodes** — `PUT` rename + restore on the **Ethereum (Base Sepolia)** baseline node (`prevItemHash`, async op polling). `POST /api/v1/nodes` is not used here (extra nodes need a matching store configuration). **Hedera (local mirror)** is also defined in `application.yml` for multi-chain baseline (intended for **[hedera-local-node](../hedera-local-node/README.md)** on the host).

The script runs **broadcasters before node `PUT`s**: on the tested image, doing node updates first can leave the revision worker in a bad state and cause broadcaster-configuration operations to fail with `NullPointerException`.

`deploy/naryo-verify/verify_naryo_api.py` is the single automated check (no ad-hoc curls required for normal use).

To **inspect filters and broadcasters** on a running Naryo instance, you can call the Configuration API directly, e.g. `curl -sS "http://127.0.0.1:6060/api/v1/filters" | jq` and `curl -sS "http://127.0.0.1:6060/api/v1/broadcasters" | jq` (port as mapped in compose). When the **Go platform API** is running with real Naryo env, **`GET http://<api-host>:8080/observability/v1/naryo/configuration`** proxies the same three list endpoints (`filters`, `broadcasters`, `broadcaster-configurations`) into one JSON payload; add **`?pipelineId=<sessionId>`** to narrow pipeline-scoped rows.

## Filters

`POST /api/v1/filters` (and several event-filter shapes we tried) return **500** on `ghcr.io/lf-decentralized-trust-labs/naryo:latest` in this environment, so the script does **not** assert filter CRUD. Broadcaster verification uses **`ALL`** destinations instead of `FILTER` + `filterId`. The **Go platform API** creates a per-pipeline filter plus a **`FILTER`** broadcaster when `POST /filters` and the async operation succeed. Some images report **`UNEXPECTED_ERROR` / `RuntimeException: onAfterApply hook error`** after the operation is enqueued; with **`NARYO_ALLOW_ALL_BROADCASTER_FALLBACK=true`** (default), the API **falls back** to an **ALL-target** HTTP broadcaster so **`POST .../start`** can still succeed (check API logs for `falling back to ALL-target broadcaster`). If you need strict FILTER-only delivery, set **`NARYO_ALLOW_ALL_BROADCASTER_FALLBACK=false`**, upgrade **`NARYO_IMAGE`**, or use a Naryo stack where filter apply hooks succeed. Revisit when upgrading Naryo.

## Baseline chains (Ethereum + Hedera)

Ethereum indexes **Base Sepolia** (chain id **84532**). The JSON-RPC URL is **`EVM_RPC_URL`** (see `deploy/naryo-verify/docker-compose.yml`, default `https://sepolia.base.org`; **no local Anvil** in this compose file). If Naryo fails at startup with **Could not subscribe to block stream**, set **`EVM_RPC_URL`** in `.env` to Alchemy, Infura, or similar. See also [Web3j: Previously installed filter has not been found](#web3j-previously-installed-filter-has-not-been-found).

Hedera follows **hedera-quickstart** ([`examples/hedera-quickstart/application.yml`](https://github.com/LF-Decentralized-Trust-labs/Naryo/blob/main/examples/hedera-quickstart/application.yml)): a **local mirror REST** URL reachable from the Naryo container, defaulting to `http://host.docker.internal:5551` (same port as **[hedera-local-node](../hedera-local-node/README.md)** mirror REST on the host). Adjust `HEDERA_MIRROR_URL` if your mirror listens elsewhere.

Set **`LOCAL_NODE_HCS_TOPIC_ID`** (or legacy **`SOLO_HCS_TOPIC_ID`**) for the declarative HCS topic filter when `docker compose up` — use a real `0.0.n` topic from your local network; **`0.0.0`** disables the filter value until you set one.

To generate **live HCS messages** on that topic for demos, run **`go run ./cmd/hcs-demo-activity`** from the repo root (see [RUNBOOK.md](./RUNBOOK.md) § Demo and e2e scripts).

Public Hedera testnet can still hit mirror/schema mismatches (e.g. newer tx types) on some Naryo builds; **hedera-local-node** on the host is the supported path for this stack.

## Web3j: `Previously installed filter has not been found`

Naryo’s **Ethereum** path uses **web3j**, which creates JSON-RPC **filters** (e.g. block or log filters via `eth_newBlockFilter` / related calls) and polls them. Logs like:

```text
WARN ... org.web3j.protocol.core.filters.Filter : Previously installed filter has not been found, trying to re-install. Filter id: ...
```

mean the RPC server **no longer has that filter id**. That is common when:

- The URL is **load-balanced** across nodes that do not share filter state.
- A **public / free** endpoint **drops** or **expires** filters quickly.
- The provider **does not reliably implement** long-lived filters.

Web3j then **creates a new filter** and continues; indexing may still work, but the warnings are noisy and can add load. If the control plane gets **`connection reset`** or **5xx** while talking to Naryo’s Configuration API, the JVM may be struggling (many reinstalls, OOM, or crashes) — check `docker logs` for the Naryo container.

**What to do:** Point **`EVM_RPC_URL`** at a **stable** endpoint that supports filter polling for your chain (Alchemy, Infura, QuickNode, etc., on a paid or filter-capable tier if needed). A **single** JSON-RPC backend avoids filter ids disappearing across load-balanced public gateways.

## Verifying Naryo is “subscribed” to your HCS topic (e.g. `0.0.1048`)

In this stack, the topic filter is **declarative** in [`deploy/naryo-verify/application.yml`](../deploy/naryo-verify/application.yml) (`filters` → `local-node-hcs-topic`). You do **not** create that subscription via `POST /api/v1/filters` for normal use (that API is flaky on the image we test — see [Filters](#filters) above). Instead you **pass the topic id into the container** at startup:

```bash
cd deploy/naryo-verify
export LOCAL_NODE_HCS_TOPIC_ID=0.0.1048   # your real topic
docker compose up -d
```

(`SOLO_HCS_TOPIC_ID` still works as a deprecated alias via compose substitution.)

Then confirm in three layers:

1. **Mirror sees the traffic** (Naryo’s Hedera node only reads the mirror; if this fails, Naryo has nothing to index):

   ```bash
   curl -sS "http://127.0.0.1:5551/api/v1/topics/0.0.1048/messages?limit=3" | head -c 2000
   ```

   You should see JSON with recent `messages` (or an empty list if the mirror has not caught up yet).

   **If `GET /api/v1/topics/{id}` returns “Topic not found” but `hcs-demo-activity` prints successful submits:** the **consensus node** accepted the transactions; the **mirror** is a separate importer/database. A 404 means that mirror has **not** recorded that topic entity yet (lag, importer stuck, or stack out of sync). Check whether the mirror at least sees the **submit** transactions (replace `@` in the logged tx id with `-`):

   ```bash
   # tx from log: 0.0.2@1775326517.903195401  →  path id: 0.0.2-1775326517-903195401
   curl -sS "http://127.0.0.1:5551/api/v1/transactions/0.0.2-1775326517-903195401"
   ```

   - If this returns **404** too, the mirror on **5551** is not ingesting the same ledger your client uses (wrong port, wrong Docker stack, or mirror not running).
   - If this returns **200** but `/topics/0.0.1048` stays missing, treat it as **mirror/topic-indexing lag or a local-node bug** — try restarting **hedera-local-node**, waiting a few minutes, or checking mirror/importer logs.

   **If the transaction GET is still 404**, check whether the mirror has **any** data at all (empty DB / broken importer / not the hedera-local-node mirror):

   ```bash
   curl -sS "http://127.0.0.1:5551/api/v1/transactions?limit=5&order=desc"
   curl -sS "http://127.0.0.1:5551/api/v1/transactions?account.id=0.0.2&limit=10&order=desc"
   ```

   When the list is **empty** while consensus accepts submits on **50211**, the record stream is not reaching this mirror (e.g. **hedera-local-node** `record-streams-uploader` / MinIO / importer path — restart the full local stack from `hedera-local-node`, confirm **both** `network-node` and **mirror** containers are healthy). **`hcs-demo-activity`** also prints a mirror URL built from the SDK transaction id on the first submit; use that exact path to rule out manual formatting mistakes.

2. **Naryo Configuration API lists the filter** (when the build exposes it):

   ```bash
   curl -sS "http://127.0.0.1:6060/api/v1/filters" | head -c 4000
   ```

   Look for your declarative filter / topic id in the payload (shape varies by Naryo version).

3. **Mongo has Hedera transaction documents** (events land in the Hedera store’s `transactions-hedera` collection):

   ```bash
   docker exec naryo-verify-mongodb mongosh naryo --quiet --eval \
     'db.getCollection("transactions-hedera").estimatedDocumentCount()'
   ```

   Run **`hcs-demo-activity`** for a few seconds, re-run the count — it should **increase** if Naryo is syncing. To spot your topic in a document:

   ```bash
   docker exec naryo-verify-mongodb mongosh naryo --quiet --eval \
     'db.getCollection("transactions-hedera").findOne({}, {transaction_id:1, name:1, consensus_timestamp:1})'
   ```

**Operational HTTP “subscription”** (delivery to a URL) is a **broadcaster** in Naryo, configured through **`POST /api/v1/broadcaster-configurations`** and **`POST /api/v1/broadcasters`** — `verify_naryo_api.py` exercises that path with target **`ALL`** (not `FILTER` + filter id). For topic-scoped delivery you would point a broadcaster at your platform ingest URL and align Naryo’s filter/broadcaster model with your image’s supported target types.

## Run

From the repo root:

```bash
cd deploy/naryo-verify
docker compose up -d
```

Wait until Naryo answers (first boot can take **1–2 minutes**, longer on `linux/arm64` with emulation):

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:6060/api/v1/nodes
```

Then:

```bash
python3 verify_naryo_api.py
```

Expect a final `OK:` line from the script (node checks always; broadcaster CRUD runs only when a **persisted** broadcaster-configuration exists — if create fails with the known NPE and the list is empty, the script skips broadcaster CRUD instead of using a client-only UUID).

## Ports

| Host | Service |
|------|---------|
| 6060 | Naryo Configuration API (`6060` → container `8060`) |
| 7070 | Mock HTTP destination (Mockoon) |
| 27017 | MongoDB (optional; for debugging) |

## Teardown

```bash
cd deploy/naryo-verify
docker compose down
```

## References

- Naryo OpenAPI: `reference/naryo/docs/configuration/api/swagger.json` (sibling repo / path on your machine)
- Operations model: `GET /api/v1/operations/{id}` → `PENDING` → `RUNNING` → `SUCCEEDED` | `FAILED`
- Updates and deletes require `prevItemHash` from the latest `GET` of that entity
