import { wrapFetchWithPayment } from "@x402/fetch";
import { x402Client } from "@x402/core/client";
import type { Network } from "@x402/core/types";
import { ExactEvmScheme, toClientEvmSigner } from "@x402/evm";
import { createPublicClient, defineChain, http, type Chain, type PublicClient, type Transport } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { baseSepolia } from "viem/chains";

export type ControlPlaneFetchOptions = {
  /** 0x-prefixed secp256k1 private key used to sign x402 (EVM exact) payments. */
  evmPrivateKey: `0x${string}`;
  /**
   * CAIP-2 network id advertised by the API (default matches Go: eip155:84532 / Base Sepolia).
   * @see internal/config/x402.go
   */
  x402Network?: string;
  /** Required for chains other than Base Sepolia (84532) unless you extend the built-in map. */
  evmRpcUrl?: string;
  /** Underlying fetch (default globalThis.fetch). */
  fetchImpl?: typeof fetch;
};

function chainFromCaip2(caip2: string, rpcUrl: string | undefined): Chain {
  const m = /^eip155:(\d+)$/i.exec(caip2.trim());
  if (!m) {
    throw new Error(`Unsupported X402_NETWORK (expected eip155:<chainId>): ${caip2}`);
  }
  const id = Number(m[1]);
  if (id === baseSepolia.id) {
    return baseSepolia;
  }
  if (!rpcUrl?.trim()) {
    throw new Error(
      `Set EVM_RPC_URL for chain id ${id} (${caip2}), or use eip155:${baseSepolia.id} (Base Sepolia).`,
    );
  }
  return defineChain({
    id,
    name: `chain-${id}`,
    nativeCurrency: { name: "Ether", symbol: "ETH", decimals: 18 },
    rpcUrls: { default: { http: [rpcUrl.trim()] } },
  });
}

function makePublicClient(
  chain: Chain,
  rpcUrl: string | undefined,
): PublicClient<Transport, Chain> {
  return createPublicClient({
    chain,
    transport: http(rpcUrl),
  });
}

/** Drop localhost RPC when paying on Base Sepolia — leftover Anvil URLs break signing against chain 84532. */
function effectiveEvmRpcUrl(x402Network: string, evmRpcUrl: string | undefined): string | undefined {
  const trimmed = evmRpcUrl?.trim();
  if (!trimmed) return undefined;
  const m = /^eip155:(\d+)$/i.exec(x402Network.trim());
  const id = m ? Number(m[1]) : 0;
  if (id !== baseSepolia.id) return trimmed;
  try {
    const u = new URL(trimmed);
    if (u.hostname === "localhost" || u.hostname === "127.0.0.1") {
      return undefined;
    }
  } catch {
    return trimmed;
  }
  return trimmed;
}

/**
 * Returns a `fetch` that implements the x402 client flow from `@x402/fetch`: first request, on 402 parse
 * requirements, sign (EVM exact), retry with PAYMENT-SIGNATURE — same as the Go payment middleware expects.
 *
 * Only `POST .../start` is x402-gated (amount = X402_PRICE × X402_START_RUNWAY_SECONDS). `GET .../status`
 * uses prepaid balance only (must be > 0 while running/paused); it is not in {@link PIPELINE_X402_ROUTE_KEYS}.
 *
 * Mirrors `middleware.PipelineRoutes` in `internal/http/middleware/x402.go`.
 * Prepaid ledger credits are separate (off-chain units); `POST .../topup/x402` is not payment-gated by the middleware.
 */
export function createControlPlaneFetch(options: ControlPlaneFetchOptions): typeof fetch {
  const {
    evmPrivateKey,
    x402Network = "eip155:84532",
    evmRpcUrl,
    fetchImpl = globalThis.fetch.bind(globalThis),
  } = options;

  const rpc = effectiveEvmRpcUrl(x402Network, evmRpcUrl);
  const chain = chainFromCaip2(x402Network, rpc);
  const account = privateKeyToAccount(evmPrivateKey);
  const publicClient = makePublicClient(chain, rpc);
  const evmSigner = toClientEvmSigner(account, publicClient);

  const client = new x402Client().register(
    x402Network as Network,
    new ExactEvmScheme(evmSigner, { rpcUrl: rpc }),
  );

  return wrapFetchWithPayment(fetchImpl, client);
}
