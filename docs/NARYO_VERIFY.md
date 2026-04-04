# Naryo Configuration API (standalone verification)

This proves **CRUD** plus **`GET /api/v1/operations/{id}`** polling for:

- **Broadcaster configurations** — create, update, delete.
- **Broadcasters** — create, update, delete with target type **`ALL`** (see below).
- **Nodes** — `PUT` rename + restore on the **Ethereum (Anvil)** baseline node (`prevItemHash`, async op polling). `POST /api/v1/nodes` is not used here (extra nodes need a matching store configuration). **Hedera (Solo)** is also defined in `application.yml` for multi-chain baseline.

The script runs **broadcasters before node `PUT`s**: on the tested image, doing node updates first can leave the revision worker in a bad state and cause broadcaster-configuration operations to fail with `NullPointerException`.

`deploy/naryo-verify/verify_naryo_api.py` is the single automated check (no ad-hoc curls required for normal use).

## Filters

`POST /api/v1/filters` (and several event-filter shapes we tried) return **500** on `ghcr.io/lf-decentralized-trust-labs/naryo:latest` in this environment, so the script does **not** assert filter CRUD. Broadcaster verification uses **`ALL`** destinations instead of `FILTER` + `filterId`. Revisit when upgrading Naryo or pointing at a stack where filter creation succeeds.

## Baseline chains (Ethereum + Hedera)

Ethereum follows the upstream **quickstart** layout ([`examples/quickstart/application.yml`](https://github.com/LF-Decentralized-Trust-labs/Naryo/blob/main/examples/quickstart/application.yml)): **Anvil** in Docker at `http://anvil:8545`, so Naryo does not depend on public RPC rate limits or `eth_newBlockFilter` support on third-party endpoints.

Hedera follows **hedera-quickstart** ([`examples/hedera-quickstart/application.yml`](https://github.com/LF-Decentralized-Trust-labs/Naryo/blob/main/examples/hedera-quickstart/application.yml)): a **local mirror** URL reachable from the Naryo container, defaulting to `http://host.docker.internal:5551` (typical Solo mirror REST after port-forward; your Solo layout may use another port — see [Solo mirror docs](https://solo.hiero.org/v0.47.0/docs/solo-with-mirror-node/)).

Set **`HEDERA_MIRROR_URL`** (and optionally **`SOLO_HCS_TOPIC_ID`** for the declarative topic filter) when `docker compose up` if defaults do not match your Solo deployment.

Public Hedera testnet can still hit mirror/schema mismatches (e.g. newer tx types) on some Naryo builds; **local Solo** is the supported path for this stack.

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

Expect a final `OK:` line from the script (broadcaster / node checks; broadcaster-configuration may be skipped on known image bugs).

## Ports

| Host | Service |
|------|---------|
| 6060 | Naryo Configuration API (`6060` → container `8060`) |
| 8545 | Anvil JSON-RPC (host; optional debugging) |
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
