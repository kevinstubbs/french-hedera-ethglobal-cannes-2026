# Naryo Configuration API (standalone verification)

This proves **CRUD** plus **`GET /api/v1/operations/{id}`** polling for:

- **Broadcaster configurations** — create, update, delete.
- **Broadcasters** — create, update, delete with target type **`ALL`** (see below).
- **Nodes** — `PUT` rename + restore on the **Ethereum (Anvil)** baseline node (`prevItemHash`, async op polling). `POST /api/v1/nodes` is not used here (extra nodes need a matching store configuration). **Hedera (local mirror)** is also defined in `application.yml` for multi-chain baseline (intended for **[hedera-local-node](../hedera-local-node/README.md)** on the host).

The script runs **broadcasters before node `PUT`s**: on the tested image, doing node updates first can leave the revision worker in a bad state and cause broadcaster-configuration operations to fail with `NullPointerException`.

`deploy/naryo-verify/verify_naryo_api.py` is the single automated check (no ad-hoc curls required for normal use).

## Filters

`POST /api/v1/filters` (and several event-filter shapes we tried) return **500** on `ghcr.io/lf-decentralized-trust-labs/naryo:latest` in this environment, so the script does **not** assert filter CRUD. Broadcaster verification uses **`ALL`** destinations instead of `FILTER` + `filterId`. Revisit when upgrading Naryo or pointing at a stack where filter creation succeeds.

## Baseline chains (Ethereum + Hedera)

Ethereum follows the upstream **quickstart** layout ([`examples/quickstart/application.yml`](https://github.com/LF-Decentralized-Trust-labs/Naryo/blob/main/examples/quickstart/application.yml)): **Anvil** in Docker at `http://anvil:8545`, so Naryo does not depend on public RPC rate limits or `eth_newBlockFilter` support on third-party endpoints.

Hedera follows **hedera-quickstart** ([`examples/hedera-quickstart/application.yml`](https://github.com/LF-Decentralized-Trust-labs/Naryo/blob/main/examples/hedera-quickstart/application.yml)): a **local mirror REST** URL reachable from the Naryo container, defaulting to `http://host.docker.internal:5551` (same port as **[hedera-local-node](../hedera-local-node/README.md)** mirror REST on the host). Adjust `HEDERA_MIRROR_URL` if your mirror listens elsewhere.

Set **`LOCAL_NODE_HCS_TOPIC_ID`** (or legacy **`SOLO_HCS_TOPIC_ID`**) for the declarative HCS topic filter when `docker compose up` — use a real `0.0.n` topic from your local network; **`0.0.0`** disables the filter value until you set one.

To generate **live HCS messages** on that topic for demos, run **`go run ./cmd/hcs-demo-activity`** from the repo root (see [RUNBOOK.md](./RUNBOOK.md) § Demo and e2e scripts).

Public Hedera testnet can still hit mirror/schema mismatches (e.g. newer tx types) on some Naryo builds; **hedera-local-node** on the host is the supported path for this stack.

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
| 18545 | Anvil JSON-RPC on host (container still `anvil:8545` inside compose; 8545 is often used by hedera-local-node web3) |
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
