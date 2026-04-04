/**
 * Demo: create pipeline → x402 start → optional synthetic Naryo ingest → poll GET status (~5s, SIGINT to stop).
 *
 * Prereqs: Go API running; API X402_PAY_TO set (non-zero). Default X402_NETWORK is Base Sepolia (eip155:84532),
 * not Ethereum Sepolia (eip155:11155111) — fund the payer key with USDC on that same network.
 * Prepaid: ledger credits are off-chain metering units (not ETH). On-chain x402 is only for POST .../start
 * (X402_PRICE per second × X402_START_RUNWAY_SECONDS). GET .../status needs prepaid balance > 0 (409 if empty).
 * If AGENT_ID is set and create returns 409, this script auto-credits via POST .../topup/x402 (plain fetch;
 * that route is not payment-gated in the Go API today). Disable with DEMO_AUTO_PREPAID_TOPUP=0.
 * Optional DEMO_PREPAID_TOPUP_MIN_UNITS (default 1000): minimum credits to add per auto top-up.
 * DEMO_POLL_INTERVAL_MS (default 5000), DEMO_POLL_MAX_ROUNDS (default 0 = until SIGINT; set 1 for a single status fetch).
 *
 * Run from agents/trading-signal: npm run demo (loads ../.env via dotenv — scripts/loadDotenv.mjs).
 *
 * Start uses [createControlPlaneFetch] (wrapFetchWithPayment): first POST may get 402, client builds
 * PAYMENT-SIGNATURE and retries — same flow as @x402/fetch. If X402_NETWORK mismatches the 402 challenge,
 * the script does one plain POST to read PAYMENT-REQUIRED, then retries the paid client on the API network.
 */
import "./loadDotenv.mjs";
import { decodePaymentRequiredHeader, decodePaymentResponseHeader } from "@x402/core/http";
import { createControlPlaneFetch } from "../dist/index.js";
import { privateKeyToAccount } from "viem/accounts";

const base = (process.env.API_BASE_URL ?? "http://127.0.0.1:8080").replace(/\/+$/, "");
const agentId = process.env.AGENT_ID ?? "";
const pk = process.env.X402_EVM_PRIVATE_KEY?.trim();
const x402NetworkEnv = process.env.X402_NETWORK?.trim() || "eip155:84532";
const evmRpcUrl = process.env.EVM_RPC_URL?.trim() || undefined;
const ingestSecret = process.env.NARYO_INGEST_SECRET?.trim();
const expectPub = process.env.AGENT_EVM_PUBLIC_ADDRESS?.trim();

if (!pk?.startsWith("0x") || pk.length < 66) {
  console.error("Set X402_EVM_PRIVATE_KEY in .env (0x-prefixed hex, 32-byte key).");
  process.exit(1);
}

/** @type {`0x${string}`} */
const evmPrivateKey = /** @type {`0x${string}`} */ (pk);

if (expectPub) {
  const derived = privateKeyToAccount(evmPrivateKey).address.toLowerCase();
  const want = expectPub.toLowerCase();
  if (derived !== want) {
    console.warn(
      `AGENT_EVM_PUBLIC_ADDRESS (${want}) does not match X402_EVM_PRIVATE_KEY (derives ${derived}).`,
    );
  }
}

const jsonHeaders = {
  Accept: "application/json",
  "Content-Type": "application/json",
};

/**
 * x402 v2 PaymentRequired uses `accepts` (not `paymentRequirements` — that name only appears in some error dumps).
 *
 * @param {Response} res
 * @returns {string | undefined}
 */
function networkFromPaymentRequired(res) {
  const hdr =
    res.headers.get("PAYMENT-REQUIRED") ??
    res.headers.get("payment-required") ??
    res.headers.get("Payment-Required");
  if (!hdr?.trim()) return undefined;
  try {
    const pr = decodePaymentRequiredHeader(hdr.trim());
    const acc = pr.accepts ?? pr.Accepts;
    const list = Array.isArray(acc) ? acc : acc != null ? [acc] : [];
    const first = list[0];
    const net = first?.network ?? first?.Network;
    return typeof net === "string" && net ? net : undefined;
  } catch {
    return undefined;
  }
}

/**
 * After a paid retry, a 402 with body `{}` is common when verify succeeded and the handler returned 2xx but
 * facilitator settlement failed — the reason is in PAYMENT-RESPONSE (base64 JSON), not the body.
 *
 * @param {Response} res
 * @returns {Record<string, unknown> | null}
 */
function settleDetailsFrom402Response(res) {
  const raw =
    res.headers.get("PAYMENT-RESPONSE") ??
    res.headers.get("payment-response") ??
    res.headers.get("X-PAYMENT-RESPONSE") ??
    res.headers.get("x-payment-response");
  if (!raw?.trim()) return null;
  try {
    const decoded = decodePaymentResponseHeader(raw.trim());
    return decoded && typeof decoded === "object" ? /** @type {Record<string, unknown>} */ (decoded) : null;
  } catch {
    return null;
  }
}

/** Plain POST to learn CAIP-2 network from a 402 PAYMENT-REQUIRED (no payment attempt). */
async function networkFromStart402Challenge(url, headers) {
  const r = await fetch(url, { method: "POST", headers, body: "{}" });
  const net = r.status === 402 ? networkFromPaymentRequired(r) : undefined;
  await r.arrayBuffer();
  return net;
}

/**
 * Unpaid POST /start: decode PAYMENT-REQUIRED and log ERC-20 + payee (one extra round-trip for demo clarity).
 *
 * @param {string} url
 * @param {Record<string, string>} headers
 */
async function logUnpaidStartPaymentDetails(url, headers) {
  const r = await fetch(url, { method: "POST", headers, body: "{}" });
  if (r.status !== 402) {
    if (r.status === 409) {
      const t = await r.text();
      console.warn("Could not log x402 challenge: /start returned 409 (prepaid?) instead of 402:", t.slice(0, 200));
    } else {
      await r.arrayBuffer();
    }
    return;
  }
  const hdr =
    r.headers.get("PAYMENT-REQUIRED") ??
    r.headers.get("payment-required") ??
    r.headers.get("Payment-Required");
  if (!hdr?.trim()) {
    await r.arrayBuffer();
    console.warn("402 on /start but no PAYMENT-REQUIRED header");
    return;
  }
  try {
    const pr = decodePaymentRequiredHeader(hdr.trim());
    const acc = pr.accepts ?? pr.Accepts;
    const list = Array.isArray(acc) ? acc : acc != null ? [acc] : [];
    const first = list[0];
    if (!first || typeof first !== "object") {
      console.warn("PAYMENT-REQUIRED has empty accepts[]");
    } else {
      /** @param {string} k */
      const p = (k) =>
        /** @type {Record<string, unknown>} */ (first)[k] ??
        /** @type {Record<string, unknown>} */ (first)[k.charAt(0).toUpperCase() + k.slice(1)];
      const asset = p("asset");
      const network = p("network");
      const payTo = p("payTo");
      const amount =
        p("maxAmountRequired") ?? p("amount") ?? p("maxAmount");
      console.log(
        "x402 PAYMENT-REQUIRED: fund ERC-20",
        String(asset),
        "on",
        String(network),
        "| payTo",
        String(payTo),
        "| amount field:",
        amount != null ? String(amount) : "(see JSON below)",
      );
      console.log("x402 first accept:", JSON.stringify(first, null, 2));
      const payToLc = String(payTo).toLowerCase();
      if (payToLc === "0x0000000000000000000000000000000000000000") {
        console.warn(
          "payTo is 0x0 — cmd/api X402_PAY_TO is unset or zero. The facilitator will try to settle USDC to the zero address and the tx reverts (invalid_exact_evm_transaction_failed). Set X402_PAY_TO to a real payee in cmd/api/.env and restart the API.",
        );
      }
    }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.warn("Could not decode PAYMENT-REQUIRED:", msg);
  }
  await r.arrayBuffer();
}

console.log("API_BASE_URL", base);
console.log("X402_NETWORK (.env)", x402NetworkEnv);

/**
 * @param {string} text response body
 * @returns {{ required: number; remaining: number } | null}
 */
function prepaidAmountsFromBody(text) {
  try {
    const j = JSON.parse(text);
    const req = j.requiredPrepaidUnits;
    const rem = j.remainingPrepaidUnits;
    if (typeof req === "number" && typeof rem === "number") {
      return { required: req, remaining: rem };
    }
  } catch {
    /* ignore */
  }
  return null;
}

/** Internal ledger credits for metering — not ETH. x402 is only for POST .../start. */
function prepaidExplainer() {
  return (
    "Ledger credits ≠ ETH: they are internal prepaid units for runtime debits. " +
    "The wallet pays on-chain for x402 on POST .../start only (X402_PRICE × X402_START_RUNWAY_SECONDS). GET .../status needs balance > 0."
  );
}

/**
 * @param {string} url
 * @param {Record<string, string>} headers
 * @param {unknown} body
 */
async function postJson(url, headers, body) {
  const r = await fetch(url, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  const text = await r.text();
  return { ok: r.ok, status: r.status, text };
}

const createBody = agentId ? { agentId } : {};
let cr = await postJson(`${base}/v1/pipelines`, jsonHeaders, createBody);

if (!cr.ok && cr.status === 409 && agentId && process.env.DEMO_AUTO_PREPAID_TOPUP !== "0") {
  const prepaid = prepaidAmountsFromBody(cr.text);
  if (prepaid) {
    const rawMin = process.env.DEMO_PREPAID_TOPUP_MIN_UNITS ?? "1000";
    const minPad = Number.parseInt(rawMin, 10);
    const pad = Number.isFinite(minPad) && minPad > 0 ? minPad : 1000;
    const amount = Math.max(prepaid.required, pad);
    console.warn(
      `Prepaid runway low (${prepaid.remaining} / ${prepaid.required} ledger credits). ${prepaidExplainer()}`,
    );
    console.warn(
      `Auto-crediting ${amount} ledger credits via POST .../topup/x402 (no PAYMENT-SIGNATURE on that route in this API).`,
    );
    const top = await postJson(
      `${base}/v1/agents/${encodeURIComponent(agentId)}/topup/x402`,
      jsonHeaders,
      {
        amountUnits: amount,
        idempotencyKey: `demo-prepaid-${agentId}-${Date.now()}`,
      },
    );
    if (!top.ok) {
      console.error("POST .../topup/x402 failed", top.status, top.text);
      process.exit(1);
    }
    cr = await postJson(`${base}/v1/pipelines`, jsonHeaders, createBody);
  }
}

if (!cr.ok) {
  console.error("POST /v1/pipelines failed", cr.status, cr.text);
  const prepaid = prepaidAmountsFromBody(cr.text);
  if (prepaid) {
    console.error(
      `Need ${prepaid.required} ledger credits, have ${prepaid.remaining}. ${prepaidExplainer()} Credit via .../topup/x402, drop DEMO_AUTO_PREPAID_TOPUP=0 to enable auto-credit (default with AGENT_ID), or set PREPAID_DEV_AUTO_CREDIT_UNITS on cmd/api.`,
    );
  }
  process.exit(1);
}

let created;
try {
  created = JSON.parse(cr.text);
} catch {
  console.error("Invalid JSON from create", cr.text);
  process.exit(1);
}
const id = created.id;
if (!id) {
  console.error("No id in create response", created);
  process.exit(1);
}
console.log("created pipeline", id);

const startUrl = `${base}/v1/pipelines/${id}/start`;

await logUnpaidStartPaymentDetails(startUrl, jsonHeaders);

// x402 flow: wrapFetchWithPayment performs POST → on 402, parse PAYMENT-REQUIRED, sign, retry with PAYMENT-SIGNATURE.
console.log("POST .../start via x402 fetch (402 → sign → retry if required)");

let effectiveNetwork = x402NetworkEnv;
/** @type {Response | undefined} */
let sr;

for (let attempt = 0; attempt < 2; attempt++) {
  const fetchPaid = createControlPlaneFetch({
    evmPrivateKey,
    x402Network: effectiveNetwork,
    evmRpcUrl,
  });
  try {
    sr = await fetchPaid(startUrl, {
      method: "POST",
      headers: jsonHeaders,
      body: "{}",
    });
    break;
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.error(msg);
    const recoverable = attempt === 0 && msg.includes("No network/scheme registered");
    if (recoverable) {
      const apiNet = await networkFromStart402Challenge(startUrl, jsonHeaders);
      if (apiNet && apiNet !== effectiveNetwork) {
        console.warn(
          `X402_NETWORK mismatch: env has ${effectiveNetwork} but API 402 advertises ${apiNet}. Retrying payer on ${apiNet} (align agents/trading-signal/.env and cmd/api/.env).`,
        );
        effectiveNetwork = apiNet;
        continue;
      }
    }
    if (/No network\/scheme registered/i.test(msg)) {
      console.error(
        "Fix: set X402_NETWORK in agents/trading-signal/.env to match cmd/api (e.g. eip155:84532 for Base Sepolia).",
      );
    }
    process.exit(1);
  }
}

if (!sr) {
  console.error("POST .../start: no response");
  process.exit(1);
}

if (sr.ok) {
  await sr.arrayBuffer();
  console.log("started (POST .../start succeeded; x402 client handled any 402 → PAYMENT-SIGNATURE → retry)");
} else {
  const t = await sr.text();
  console.error("POST .../start failed", sr.status, t);
  const prepaid = prepaidAmountsFromBody(t);
  if (prepaid) {
    console.error(
      `Prepaid gate: need ${prepaid.required} ledger credits, have ${prepaid.remaining}. ${prepaidExplainer()}`,
    );
  }
  if (sr.status === 402) {
    const settle = settleDetailsFrom402Response(sr);
    if (settle) {
      console.error("PAYMENT-RESPONSE (what the server/facilitator reported):", JSON.stringify(settle, null, 2));
    } else {
      console.error("402 with no decodable PAYMENT-RESPONSE header (check response headers in DevTools or curl -v).");
    }
    console.error(
      [
        "The @x402/fetch client did see 402 and retried with PAYMENT-SIGNATURE. A second 402 usually means:",
        "  • verify passed, POST /start handler returned success, then on-chain settlement failed (facilitator / USDC / network).",
        "  • Empty JSON body `{}` is normal here — read PAYMENT-RESPONSE above for errorReason.",
        "Coinbase x402 + the public facilitator target Base Sepolia (84532), not Ethereum Sepolia (11155111).",
        "Fund USDC on Base Sepolia for the wallet that matches X402_EVM_PRIVATE_KEY; check API X402_FACILITATOR_URL and server logs.",
      ].join("\n"),
    );
  }
  if (sr.status === 409 && prepaid) {
    console.error(
      "409 before a successful paid /start: prepaid rejected the handler before or after x402 (ensure ledger runway; see PREPAID_DEV_AUTO_CREDIT_UNITS).",
    );
  } else if (sr.status >= 400 && sr.status < 500 && sr.status !== 402) {
    console.error("If prepaid error: set PREPAID_DEV_AUTO_CREDIT_UNITS on API or top up the agent.");
  }
  process.exit(1);
}

if (ingestSecret) {
  const eventId = `demo-${Date.now()}`;
  const ir = await fetch(`${base}/internal/naryo/v1/events`, {
    method: "POST",
    headers: {
      ...jsonHeaders,
      "X-Naryo-Webhook-Secret": ingestSecret,
    },
    body: JSON.stringify({
      sessionId: id,
      eventId,
      payload: { source: "demo-control-plane", at: new Date().toISOString() },
    }),
  });
  if (!ir.ok) {
    console.warn("ingest failed (optional)", ir.status, await ir.text());
  } else {
    const ing = await ir.json();
    console.log("ingest", ing);
  }
} else {
  console.log("skip ingest (set NARYO_INGEST_SECRET to POST /internal/naryo/v1/events)");
}

const pollMsRaw = process.env.DEMO_POLL_INTERVAL_MS ?? "5000";
const pollMs = Number.parseInt(pollMsRaw, 10);
const pollIntervalMs = Number.isFinite(pollMs) && pollMs >= 200 ? pollMs : 5000;
const maxRoundsRaw = process.env.DEMO_POLL_MAX_ROUNDS ?? "0";
const maxRounds = Number.parseInt(maxRoundsRaw, 10);
const pollUntilSigint = !Number.isFinite(maxRounds) || maxRounds <= 0;

/** @type {Set<string>} */
const seenEventIds = new Set();
let stopPoll = false;
process.on("SIGINT", () => {
  stopPoll = true;
  console.log("\nStopping status poll (SIGINT).");
});

console.log(
  `Polling GET .../status every ~${pollIntervalMs}ms` +
    (pollUntilSigint ? " until SIGINT" : `, max ${maxRounds} round(s)`) +
    " (plain fetch — not x402; 409 if prepaid hits zero while running).",
);

let round = 0;
while (!stopPoll) {
  const st = await fetch(`${base}/v1/pipelines/${id}/status`, {
    headers: { Accept: "application/json" },
  });
  if (!st.ok) {
    const t = await st.text();
    console.error("GET .../status failed", st.status, t);
    if (st.status === 409) {
      console.error("Prepaid empty while pipeline is running/paused — top up ledger (POST .../topup/x402) or wait for dev auto-credit.");
    }
    process.exit(1);
  }
  const status = /** @type {Record<string, unknown>} */ (await st.json());
  const evs = status.recentNaryoEvents;
  if (Array.isArray(evs)) {
    for (const e of evs) {
      if (!e || typeof e !== "object") continue;
      const rec = /** @type {Record<string, unknown>} */ (e);
      const eid = rec.eventId ?? rec.event_id;
      if (typeof eid !== "string" || seenEventIds.has(eid)) continue;
      seenEventIds.add(eid);
      console.log("new Naryo event", eid, JSON.stringify(e));
    }
  }
  console.log(
    "status tick",
    new Date().toISOString(),
    "state",
    status.state,
    "prepaidBalanceUnits",
    status.prepaidBalanceUnits ?? "(n/a)",
  );
  round++;
  if (!pollUntilSigint && round >= maxRounds) {
    break;
  }
  await new Promise((r) => setTimeout(r, pollIntervalMs));
}
