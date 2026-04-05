/**
 * Demo: create pipeline → PUT reconfigure (event subscription intent) → x402 start → poll GET .../status (~5s, SIGINT to stop).
 * `recentNaryoEvents` on status only appear when something (e.g. Naryo HTTP broadcaster) POSTs `/internal/naryo/v1/events` — this script does not simulate that.
 *
 * After create, the script **asks the control plane** for a pipeline whose session `config.eventSubscriptions`
 * describes a logical **OR** of:
 *   (1) any HCS messages on Hedera topic `DEMO_HCS_TOPIC_ID` (default 0.0.8510924) on `DEMO_HEDERA_NETWORK` (default testnet), and
 *   (2) USDC ERC-20 `Transfer` on `DEMO_LISTEN_EVM_CAIP2` (default = `X402_NETWORK`, usually Base Sepolia eip155:84532)
 *       from `DEMO_USDC_TRANSFER_FROM` (default demo wallet below) for contract `DEMO_USDC_CONTRACT` (default Base Sepolia USDC).
 *
 * Env overrides (see agents/trading-signal/.env.example):
 *   DEMO_HCS_TOPIC_ID         — Hedera topic
 *   DEMO_HEDERA_NETWORK       — e.g. testnet
 *   DEMO_USDC_TRANSFER_FROM   — `from` on Transfer
 *   DEMO_USDC_CONTRACT        — USDC on that EVM chain
 *   DEMO_LISTEN_EVM_CAIP2     — CAIP-2 for the USDC rule (default X402_NETWORK)
 *
 * **Scope:** This only records subscription **intent** on the session (dashboard / agents / ops). Naryo must still
 * index the same topic and chain — e.g. `deploy/naryo-verify` sets `HEDERA_MIRROR_URL`, `LOCAL_NODE_HCS_TOPIC_ID`,
 * and `EVM_RPC_URL` (default Base Sepolia `https://sepolia.base.org`). See docs/PIPELINE_EVENT_ROUTING.md.
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
import { getAddress } from "viem";
import { privateKeyToAccount } from "viem/accounts";

const base = (process.env.API_BASE_URL ?? "http://127.0.0.1:8080").replace(/\/+$/, "");
const agentId = process.env.AGENT_ID ?? "";
const pk = process.env.X402_EVM_PRIVATE_KEY?.trim();
const x402NetworkEnv = process.env.X402_NETWORK?.trim() || "eip155:84532";
const evmRpcUrl = process.env.EVM_RPC_URL?.trim() || undefined;
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

const demoPipelineSubs = resolveDemoPipelineSubscriptionInputs(x402NetworkEnv);
logDemoPipelineIntentBanner(demoPipelineSubs);

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

/**
 * @param {string} url
 * @param {Record<string, string>} headers
 * @param {unknown} body
 */
async function putJson(url, headers, body) {
  const r = await fetch(url, {
    method: "PUT",
    headers,
    body: JSON.stringify(body),
  });
  const text = await r.text();
  return { ok: r.ok, status: r.status, text };
}

/**
 * Resolved env for `config.eventSubscriptions` (single source for banner + PUT body).
 *
 * @param {string} x402NetworkFallback used when DEMO_LISTEN_EVM_CAIP2 is unset
 */
function resolveDemoPipelineSubscriptionInputs(x402NetworkFallback) {
  const topicId = process.env.DEMO_HCS_TOPIC_ID?.trim() || "0.0.8510924";
  const hederaNetwork = process.env.DEMO_HEDERA_NETWORK?.trim() || "testnet";
  const transferFrom =
    process.env.DEMO_USDC_TRANSFER_FROM?.trim() || "0xefd3c8d378aaa06c6c349f704680fc5d7c61a51d";
  const usdcContract =
    process.env.DEMO_USDC_CONTRACT?.trim() || "0x036CbD53842c5426634e7929541eC2318f3dCF7e";
  const listenEvmCaip2 = process.env.DEMO_LISTEN_EVM_CAIP2?.trim() || x402NetworkFallback;
  return { topicId, hederaNetwork, transferFrom, usdcContract, listenEvmCaip2 };
}

/**
 * @param {ReturnType<typeof resolveDemoPipelineSubscriptionInputs>} inputs
 */
function buildEventSubscriptionsFromInputs(inputs) {
  return {
    version: 1,
    match: "any",
    description:
      "Deliver events when ANY subscription matches: Hedera HCS messages on the topic, or USDC Transfer from the given address.",
    subscriptions: [
      {
        id: "hedera-hcs-topic-messages",
        kind: "hedera_hcs_topic",
        hederaNetwork: inputs.hederaNetwork,
        topicId: inputs.topicId,
      },
      {
        id: "evm-usdc-transfer-from",
        kind: "erc20_transfer",
        caip2: inputs.listenEvmCaip2,
        contractAddress: getAddress(inputs.usdcContract),
        tokenSymbol: "USDC",
        fromAddress: getAddress(inputs.transferFrom),
      },
    ],
  };
}

/**
 * @param {ReturnType<typeof resolveDemoPipelineSubscriptionInputs>} inputs
 */
function logDemoPipelineIntentBanner(inputs) {
  const fromAddr = getAddress(inputs.transferFrom);
  const usdcAddr = getAddress(inputs.usdcContract);
  console.log("");
  console.log("=== Pipeline subscription intent (will apply via PUT .../reconfigure → config.eventSubscriptions) ===");
  console.log("Match ANY of: HCS messages on Hedera topic | USDC Transfer from address on EVM network.");
  console.log("");
  console.log("  DEMO_HCS_TOPIC_ID           →", inputs.topicId);
  console.log("  DEMO_HEDERA_NETWORK         →", inputs.hederaNetwork);
  console.log("  DEMO_USDC_TRANSFER_FROM     →", fromAddr);
  console.log("  DEMO_USDC_CONTRACT          →", usdcAddr);
  console.log("  DEMO_LISTEN_EVM_CAIP2       →", inputs.listenEvmCaip2);
  console.log("");
  console.log(
    "Scope: intent on the session only. Align Naryo (e.g. deploy/naryo-verify: HEDERA_MIRROR_URL, LOCAL_NODE_HCS_TOPIC_ID, EVM_RPC_URL).",
  );
  console.log("");
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

/** @type {ReturnType<typeof buildEventSubscriptionsFromInputs>} */
let eventSubscriptions;
try {
  eventSubscriptions = buildEventSubscriptionsFromInputs(demoPipelineSubs);
} catch (e) {
  const msg = e instanceof Error ? e.message : String(e);
  console.error("Invalid DEMO_USDC_TRANSFER_FROM or DEMO_USDC_CONTRACT (must be valid EVM addresses):", msg);
  process.exit(1);
}

const rc = await putJson(`${base}/v1/pipelines/${id}/reconfigure`, jsonHeaders, {
  patch: { eventSubscriptions },
});
if (!rc.ok) {
  console.error("PUT .../reconfigure (eventSubscriptions) failed", rc.status, rc.text);
  process.exit(1);
}
console.log("Applied subscription intent to session (OR):");
console.log(
  "  • Hedera",
  eventSubscriptions.subscriptions[0].hederaNetwork,
  "topic",
  eventSubscriptions.subscriptions[0].topicId,
);
console.log(
  "  • USDC Transfer from",
  eventSubscriptions.subscriptions[1].fromAddress,
  "on",
  eventSubscriptions.subscriptions[1].caip2,
);
console.log("full config.eventSubscriptions:", JSON.stringify(eventSubscriptions, null, 2));

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
    if (/naryo:.*NullPointerException/i.test(t)) {
      console.error(
        [
          "Naryo broadcaster-configuration async NPE: ensure GET /api/v1/broadcaster-configurations has an HTTP row whose endpoint.url matches NARYO_PLATFORM_INGEST_URL (API auto-reuses), or set NARYO_BROADCASTER_CONFIGURATION_ID.",
          "Fresh Mongo + broken image: docker compose pull in deploy/naryo-verify, or set NARYO_IMAGE in deploy/naryo-verify/.env — see deploy/naryo-verify/.env.example.",
          "API startup requires NARYO_CONFIG_API_BASE and NARYO_PLATFORM_INGEST_URL; verify with deploy/naryo-verify/verify_naryo_api.py and RUNBOOK.",
        ].join("\n"),
      );
    } else {
      console.error("If prepaid error: set PREPAID_DEV_AUTO_CREDIT_UNITS on API or top up the agent.");
    }
  }
  process.exit(1);
}

const pollMsRaw = process.env.DEMO_POLL_INTERVAL_MS ?? "5000";
const pollMs = Number.parseInt(pollMsRaw, 10);
const pollIntervalMs = Number.isFinite(pollMs) && pollMs >= 200 ? pollMs : 5000;
const maxRoundsRaw = process.env.DEMO_POLL_MAX_ROUNDS ?? "0";
const maxRounds = Number.parseInt(maxRoundsRaw, 10);
const pollUntilSigint = !Number.isFinite(maxRounds) || maxRounds <= 0;

let stopPoll = false;
process.on("SIGINT", () => {
  stopPoll = true;
  console.log("\nStopping status poll (SIGINT).");
});

const statusUrl = `${base}/v1/pipelines/${id}/status`;
console.log(
  `Polling ${statusUrl} every ~${pollIntervalMs}ms` +
    (pollUntilSigint ? " until SIGINT" : `, max ${maxRounds} round(s)`) +
    " (plain fetch — not x402; 409 if prepaid hits zero while running).",
);

let round = 0;
while (!stopPoll) {
  const t0 = new Date().toISOString();
  console.log(`[${t0}] GET ${statusUrl}`);
  const st = await fetch(statusUrl, {
    headers: { Accept: "application/json" },
  });
  const bodyText = await st.text();
  if (!st.ok) {
    console.error(`[${t0}] HTTP ${st.status} body:`, bodyText);
    if (st.status === 409) {
      console.error("Prepaid empty while pipeline is running/paused — top up ledger (POST .../topup/x402) or wait for dev auto-credit.");
    }
    process.exit(1);
  }
  let status;
  try {
    status = /** @type {Record<string, unknown>} */ (JSON.parse(bodyText));
  } catch {
    console.error(`[${t0}] HTTP ${st.status} non-JSON body:`, bodyText);
    process.exit(1);
  }
  console.log(`[${t0}] HTTP ${st.status} JSON:`, JSON.stringify(status, null, 2));
  round++;
  if (!pollUntilSigint && round >= maxRounds) {
    break;
  }
  await new Promise((r) => setTimeout(r, pollIntervalMs));
}
