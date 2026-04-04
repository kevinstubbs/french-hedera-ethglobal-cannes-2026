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

/**
 * Returns a `fetch` that retries on HTTP 402 with a PAYMENT-SIGNATURE header (x402, EVM exact),
 * consistent with the Go API’s x402 payment middleware.
 *
 * Routes that never return 402 (e.g. `GET /v1/pipelines/{id}/status`) behave like plain fetch.
 *
 * For the list of paid mutations, see {@link PIPELINE_X402_ROUTE_KEYS} in `./pipelineX402Routes.js`
 * (mirrors `middleware.PipelineRoutes` in `internal/http/middleware/x402.go`).
 */
export function createControlPlaneFetch(options: ControlPlaneFetchOptions): typeof fetch {
  const {
    evmPrivateKey,
    x402Network = "eip155:84532",
    evmRpcUrl,
    fetchImpl = globalThis.fetch.bind(globalThis),
  } = options;

  const chain = chainFromCaip2(x402Network, evmRpcUrl);
  const account = privateKeyToAccount(evmPrivateKey);
  const publicClient = makePublicClient(chain, evmRpcUrl);
  const evmSigner = toClientEvmSigner(account, publicClient);

  const client = new x402Client().register(
    x402Network as Network,
    new ExactEvmScheme(evmSigner, { rpcUrl: evmRpcUrl }),
  );

  return wrapFetchWithPayment(fetchImpl, client);
}
