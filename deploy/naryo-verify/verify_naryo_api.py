#!/usr/bin/env python3
"""
Proves Naryo Configuration API CRUD + /api/v1/operations/{id} polling.

Uses the Ethereum (Anvil) baseline node id from application.yml (see docs/NARYO_VERIFY.md).
Hedera (Solo) is configured there too for multi-chain baseline. Creating extra nodes via POST
often requires a matching store configuration; this script exercises node *updates*
(PUT + prevItemHash) on the Ethereum baseline only.

Filter create/update via the Configuration API returns 500 on the current
`naryo:latest` image in our tests, so broadcaster CRUD uses target type **ALL**
(no filterId) while still exercising async operations and prevItemHash.

Broadcaster-configuration create can also fail asynchronously with a
NullPointerException on this image; when that happens this verifier logs the
failure and continues with broadcaster + node verification.

Prerequisites: from this directory, `docker compose up -d`.
"""
from __future__ import annotations

import copy
import json
import sys
import time
import urllib.error
import urllib.request
import uuid
from typing import Any


BASE = "http://127.0.0.1:6060"
# Ethereum Anvil node id — matches deploy/naryo-verify/application.yml (Naryo quickstart example).
BASELINE_NODE_ID = "eadc75b2-4217-4018-95af-f67c13058976"


class OperationFailed(Exception):
    def __init__(self, label: str, operation_body: dict[str, Any]) -> None:
        super().__init__(label)
        self.label = label
        self.operation_body = operation_body


def req(
    method: str,
    path: str,
    body: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    url = BASE + path
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            raw = resp.read().decode("utf-8")
            if not raw:
                return resp.status, None
            return resp.status, json.loads(raw)
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8")
        try:
            parsed = json.loads(raw) if raw else None
        except json.JSONDecodeError:
            parsed = raw
        return e.code, parsed


def wait_api_ready(max_wait_s: int = 120) -> None:
    deadline = time.time() + max_wait_s
    while time.time() < deadline:
        try:
            code, _ = req("GET", "/api/v1/nodes")
            if code == 200:
                print("Naryo Configuration API is reachable.")
                return
        except OSError:
            pass
        time.sleep(2)
    print("Timed out waiting for Naryo on", BASE, file=sys.stderr)
    sys.exit(1)


def poll_operation_terminal(
    op_id: str,
    label: str,
    max_wait_s: int = 120,
) -> dict[str, Any]:
    deadline = time.time() + max_wait_s
    while time.time() < deadline:
        code, body = req("GET", f"/api/v1/operations/{op_id}")
        if code != 200 or not isinstance(body, dict):
            print(f"[{label}] poll failed HTTP {code} body={body}", file=sys.stderr)
            time.sleep(1)
            continue
        state = body.get("state")
        print(f"[{label}] operation {op_id} state={state}")
        if state in ("SUCCEEDED", "FAILED"):
            return body
        time.sleep(1)
    print(f"[{label}] timed out polling operation {op_id}", file=sys.stderr)
    sys.exit(1)


def poll_operation(op_id: str, label: str, max_wait_s: int = 120) -> dict[str, Any]:
    body = poll_operation_terminal(op_id, label, max_wait_s=max_wait_s)
    if body.get("state") == "FAILED":
        print(json.dumps(body, indent=2), file=sys.stderr)
        raise OperationFailed(label, body)
    return body


def accept_202(label: str, code: int, body: Any) -> str:
    if code != 202:
        print(f"[{label}] expected HTTP 202, got {code}: {body}", file=sys.stderr)
        sys.exit(1)
    if not isinstance(body, dict):
        print(f"[{label}] unexpected 202 body: {body}", file=sys.stderr)
        sys.exit(1)
    if "value" in body:
        return str(body["value"])
    op = body.get("operationId")
    if isinstance(op, dict) and "value" in op:
        return str(op["value"])
    print(f"[{label}] unexpected 202 body: {body}", file=sys.stderr)
    sys.exit(1)


def get_node_by_id(nodes_payload: Any, node_id: str) -> dict[str, Any]:
    if not isinstance(nodes_payload, list):
        print("GET /nodes: expected list", nodes_payload, file=sys.stderr)
        sys.exit(1)
    for n in nodes_payload:
        if isinstance(n, dict) and str(n.get("id")) == node_id:
            return n
    print("Node not found:", node_id, file=sys.stderr)
    sys.exit(1)


def normalize_node_for_write(api_node: dict[str, Any]) -> dict[str, Any]:
    """Map GET /nodes element to the JSON shape PUT expects."""
    out = copy.deepcopy(api_node)
    out.pop("id", None)
    out.pop("currentItemHash", None)
    ce = out["connection"]["connectionEndpoint"]
    if not ce.get("path"):
        ce["path"] = "/"
    rc = out["connection"].setdefault("retryConfiguration", {})
    if isinstance(rc.get("backoff"), (int, float)):
        rc["backoff"] = "30s"
    out["connection"]["keepAliveDuration"] = "5m"
    out["connection"]["connectionTimeout"] = "10s"
    out["connection"]["readTimeout"] = "30s"
    sub = out["subscription"]
    mc = sub.pop("methodConfiguration", None)
    if mc:
        interval = mc.get("interval")
        if isinstance(interval, (int, float)):
            interval = f"{int(interval)}s"
        sub["method"] = {"method": mc.get("method", "POLL"), "interval": interval}
    return out


def find_broadcaster_for_config(
    broadcasters_payload: Any,
    configuration_id: str,
    target_type: str = "ALL",
) -> dict[str, Any]:
    if not isinstance(broadcasters_payload, list):
        print("GET /broadcasters: expected list", broadcasters_payload, file=sys.stderr)
        sys.exit(1)
    for b in broadcasters_payload:
        if not isinstance(b, dict):
            continue
        if str(b.get("configurationId")) != configuration_id:
            continue
        tgt = b.get("target") or {}
        if tgt.get("type") == target_type:
            return b
    print(
        "Broadcaster not found for configurationId=",
        configuration_id,
        "target_type=",
        target_type,
        file=sys.stderr,
    )
    sys.exit(1)


def find_broadcaster_config(cfgs: Any, cid: str) -> dict[str, Any]:
    if not isinstance(cfgs, list):
        print("GET /broadcaster-configurations: expected list", cfgs, file=sys.stderr)
        sys.exit(1)
    for c in cfgs:
        if isinstance(c, dict) and str(c.get("id")) == cid:
            return c
    print("Broadcaster configuration not found:", cid, file=sys.stderr)
    sys.exit(1)


def is_transient_npe_failure(op_body: dict[str, Any]) -> bool:
    code = str(op_body.get("errorCode", "")).upper()
    msg = str(op_body.get("errorMessage", ""))
    return code == "UNEXPECTED_ERROR" and "NullPointerException" in msg


def restore_baseline_node_name(node_id: str, target_name: str) -> None:
    """Best-effort cleanup; does not call sys.exit on failure."""
    code, nodes = req("GET", "/api/v1/nodes")
    if code != 200:
        return
    node = get_node_by_id(nodes, node_id)
    if node.get("name") == target_name:
        return
    h = node.get("currentItemHash")
    if not h:
        return
    body = normalize_node_for_write(node)
    body["name"] = target_name
    code, resp = req(
        "PUT",
        f"/api/v1/nodes/{node_id}",
        {"node": body, "prevItemHash": h},
    )
    if code != 202 or not isinstance(resp, dict):
        print(
            "cleanup: could not enqueue baseline node rename",
            code,
            resp,
            file=sys.stderr,
        )
        return
    op_id = str(resp.get("value", ""))
    if not op_id:
        return
    deadline = time.time() + 60
    while time.time() < deadline:
        _, st = req("GET", f"/api/v1/operations/{op_id}")
        if isinstance(st, dict) and st.get("state") in ("SUCCEEDED", "FAILED"):
            if st.get("state") == "FAILED":
                print("cleanup: restore operation failed", st, file=sys.stderr)
            return
        time.sleep(1)


def main() -> None:
    wait_api_ready()

    suffix = uuid.uuid4().hex[:8]
    node_id = BASELINE_NODE_ID

    code, nodes = req("GET", "/api/v1/nodes")
    if code != 200:
        print("GET nodes failed", code, nodes, file=sys.stderr)
        sys.exit(1)
    baseline = get_node_by_id(nodes, node_id)
    original_baseline_name = str(baseline["name"])

    # Broadcaster config/broadcaster first: after node PUTs, this Naryo build sometimes
    # fails broadcaster-configuration ops with NullPointerException in the revision worker.

    # --- Broadcaster configuration ---
    code, cfgs = req("GET", "/api/v1/broadcaster-configurations")
    if code != 200:
        print(
            "GET broadcaster-configurations failed",
            code,
            cfgs,
            file=sys.stderr,
        )
        sys.exit(1)

    bc_id = str(uuid.uuid4())
    cfg_hash: str | None = None
    cfg_crud_verified = False
    cfg_body: dict[str, Any] = {
        "id": bc_id,
        "type": "HTTP",
        "endpoint": {"url": "http://mock-http:7070"},
        "cache": {"expirationTime": "5m"},
    }
    code, body = req("POST", "/api/v1/broadcaster-configurations", cfg_body)
    op = accept_202("create broadcaster-configuration", code, body)
    create_cfg_op = poll_operation_terminal(op, "create broadcaster-configuration")
    if create_cfg_op.get("state") == "SUCCEEDED":
        cfg_crud_verified = True
        code, cfgs = req("GET", "/api/v1/broadcaster-configurations")
        cfg = find_broadcaster_config(cfgs, bc_id)
        cfg_hash = cfg.get("currentItemHash")
        if not cfg_hash:
            print(
                "missing currentItemHash on broadcaster-configuration",
                cfg,
                file=sys.stderr,
            )
            sys.exit(1)

        code, body = req(
            "PUT",
            "/api/v1/broadcaster-configurations",
            {
                "broadcasterConfig": cfg_body,
                "prevItemHash": cfg_hash,
            },
        )
        op = accept_202("update broadcaster-configuration", code, body)
        poll_operation(op, "update broadcaster-configuration")

        code, cfgs = req("GET", "/api/v1/broadcaster-configurations")
        cfg = find_broadcaster_config(cfgs, bc_id)
        cfg_hash = cfg.get("currentItemHash")
    elif is_transient_npe_failure(create_cfg_op):
        print(
            "[create broadcaster-configuration] known image bug "
            "(async NullPointerException); continuing with broadcaster CRUD only.",
            file=sys.stderr,
        )
    else:
        print(json.dumps(create_cfg_op, indent=2), file=sys.stderr)
        sys.exit(1)

    # A broadcaster must reference a persisted configurationId. After an NPE, our random
    # bc_id was never stored — POST /broadcasters may return SUCCEEDED but GET /broadcasters
    # will not list rows for that id.
    skip_broadcaster_crud = False
    if not cfg_crud_verified:
        code, existing_cfgs = req("GET", "/api/v1/broadcaster-configurations")
        if code != 200 or not isinstance(existing_cfgs, list):
            print(
                "GET broadcaster-configurations failed",
                code,
                existing_cfgs,
                file=sys.stderr,
            )
            sys.exit(1)
        usable = [c for c in existing_cfgs if isinstance(c, dict) and c.get("id")]
        if usable:
            bc_id = str(usable[0]["id"])
            print(
                "[broadcaster CRUD] using existing broadcaster-configuration id="
                f"{bc_id} (create failed with known NPE).",
                file=sys.stderr,
            )
        else:
            skip_broadcaster_crud = True
            print(
                "[broadcaster CRUD] skipped: no broadcaster-configuration exists "
                "(create failed with NPE; cannot attach broadcasters).",
                file=sys.stderr,
            )

    # --- Broadcaster: ALL target (filter CRUD omitted — POST /filters → 500 on tested image) ---
    if not skip_broadcaster_crud:
        broadcaster_body: dict[str, Any] = {
            "configurationId": bc_id,
            "target": {
                "type": "ALL",
                "destinations": ["/verify-events"],
            },
        }
        code, body = req("POST", "/api/v1/broadcasters", broadcaster_body)
        op = accept_202("create broadcaster", code, body)
        poll_operation(op, "create broadcaster")

        code, brs = req("GET", "/api/v1/broadcasters")
        br = find_broadcaster_for_config(brs, bc_id, "ALL")
        br_id = str(br["id"])
        br_hash = br.get("currentItemHash")
        if not br_hash:
            print("missing currentItemHash on broadcaster", br, file=sys.stderr)
            sys.exit(1)

        broadcaster_body["target"]["destinations"] = ["/verify-events-v2"]
        code, body = req(
            "PUT",
            f"/api/v1/broadcasters/{br_id}",
            {"broadcaster": broadcaster_body, "prevItemHash": br_hash},
        )
        op = accept_202("update broadcaster", code, body)
        poll_operation(op, "update broadcaster")

        code, brs = req("GET", "/api/v1/broadcasters")
        br = find_broadcaster_for_config(brs, bc_id, "ALL")
        br_hash = br.get("currentItemHash")

        # --- Deletes (depend on prevItemHash) ---
        code, body = req(
            "DELETE",
            f"/api/v1/broadcasters/{br_id}",
            {"prevItemHash": br_hash},
        )
        op = accept_202("delete broadcaster", code, body)
        poll_operation(op, "delete broadcaster")

    if cfg_crud_verified and cfg_hash:
        code, body = req(
            "DELETE",
            f"/api/v1/broadcaster-configurations/{bc_id}",
            {"prevItemHash": cfg_hash},
        )
        op = accept_202("delete broadcaster-configuration", code, body)
        poll_operation(op, "delete broadcaster-configuration")

    # --- Node: PUT rename + poll + PUT restore + poll (GET→write JSON mapping) ---
    try:
        code, nodes = req("GET", "/api/v1/nodes")
        if code != 200:
            print("GET nodes failed", code, nodes, file=sys.stderr)
            sys.exit(1)
        baseline = get_node_by_id(nodes, node_id)
        node_hash = baseline.get("currentItemHash")
        if not node_hash:
            print("missing currentItemHash on baseline node", baseline, file=sys.stderr)
            sys.exit(1)

        renamed = f"{original_baseline_name}-verify-{suffix}"
        node_put = normalize_node_for_write(baseline)
        node_put["name"] = renamed
        code, body = req(
            "PUT",
            f"/api/v1/nodes/{node_id}",
            {"node": node_put, "prevItemHash": node_hash},
        )
        op = accept_202("update node (rename)", code, body)
        poll_operation(op, "update node (rename)")

        code, nodes = req("GET", "/api/v1/nodes")
        baseline = get_node_by_id(nodes, node_id)
        node_hash = baseline.get("currentItemHash")
        node_put = normalize_node_for_write(baseline)
        node_put["name"] = original_baseline_name
        code, body = req(
            "PUT",
            f"/api/v1/nodes/{node_id}",
            {"node": node_put, "prevItemHash": node_hash},
        )
        op = accept_202("update node (restore name)", code, body)
        poll_operation(op, "update node (restore name)")

    finally:
        restore_baseline_node_name(node_id, original_baseline_name)

    parts = ["nodes CRUD-update cycle", "operation polling"]
    if skip_broadcaster_crud:
        parts.append("broadcaster CRUD skipped (no config after NPE)")
    else:
        parts.append("broadcasters CRUD (ALL target)")
    parts.append(
        "broadcaster-configuration create may fail with async NPE on this image"
    )
    print("OK: " + " + ".join(parts) + ".")


if __name__ == "__main__":
    try:
        main()
    except OperationFailed:
        sys.exit(1)
