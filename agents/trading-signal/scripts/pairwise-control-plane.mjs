/**
 * Minimal x402 retry client mirroring internal/x402test.AutoPayTransport (Go).
 * Used by Go tests for agent→API pairwise HTTP coverage without a live chain RPC.
 */
const base = process.argv[2];
if (!base) {
  console.error("usage: node pairwise-control-plane.mjs <controlPlaneBaseUrl>");
  process.exit(1);
}

/**
 * @param {string} url
 * @param {RequestInit} init
 */
async function fetchWithAutoPay(url, init = {}) {
  const firstBody =
    init.body !== undefined && init.body !== null
      ? typeof init.body === "string"
        ? init.body
        : Buffer.from(await new Response(init.body).arrayBuffer()).toString()
      : "";

  const doReq = async (headers) => {
    /** @type {RequestInit} */
    const next = { ...init, headers: new Headers(init.headers) };
    for (const [k, v] of headers.entries()) {
      next.headers.set(k, v);
    }
    if (firstBody) {
      next.body = firstBody;
    }
    return fetch(url, next);
  };

  let r = await doReq(new Headers());
  if (r.status !== 402) {
    return r;
  }
  await r.arrayBuffer();
  const prHdr = r.headers.get("PAYMENT-REQUIRED");
  if (!prHdr) {
    return r;
  }
  const raw = Buffer.from(prHdr, "base64");
  const pr = JSON.parse(raw.toString());
  const accepts = pr.accepts ?? pr.Accepts;
  if (!accepts?.length) {
    return r;
  }
  const acc = { ...accepts[0] };
  acc.extra = { ...(acc.extra ?? acc.Extra ?? {}), resourceUrl: url };
  const pp = {
    x402Version: 2,
    payload: { sig: "test" },
    accepted: acc,
  };
  const sig = Buffer.from(JSON.stringify(pp)).toString("base64");
  const h = new Headers(init.headers);
  h.set("PAYMENT-SIGNATURE", sig);
  if (!h.has("Accept")) {
    h.set("Accept", "application/json");
  }
  if (!h.has("Content-Type") && firstBody) {
    h.set("Content-Type", "application/json");
  }
  return doReq(h);
}

const root = base.replace(/\/+$/, "");
const create = await fetchWithAutoPay(`${root}/v1/pipelines`, {
  method: "POST",
  headers: { "Content-Type": "application/json", Accept: "application/json" },
  body: JSON.stringify({ agentId: "pairwise-agent" }),
});
if (!create.ok) {
  console.error("create failed", create.status, await create.text());
  process.exit(1);
}
const { id } = await create.json();
if (!id) {
  console.error("no session id");
  process.exit(1);
}

const start = await fetchWithAutoPay(`${root}/v1/pipelines/${id}/start`, {
  method: "POST",
  headers: { Accept: "application/json", "Content-Type": "application/json" },
  body: "{}",
});
if (!start.ok) {
  console.error("start failed", start.status, await start.text());
  process.exit(1);
}

const st = await fetch(`${root}/v1/pipelines/${id}/status`, {
  headers: { Accept: "application/json" },
});
if (!st.ok) {
  console.error("status failed", st.status, await st.text());
  process.exit(1);
}
const j = await st.json();
if (j.state !== "running") {
  console.error("expected running", j);
  process.exit(1);
}

console.log("PAIRWISE_OK", id);
