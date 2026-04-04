import { describe, expect, it, vi } from "vitest";

vi.mock("@x402/fetch", () => ({
  wrapFetchWithPayment: vi.fn((fetchImpl: typeof fetch) => fetchImpl),
}));

import { wrapFetchWithPayment } from "@x402/fetch";
import { createControlPlaneFetch } from "./createControlPlaneFetch.js";

/** Well-known Anvil dev key (public testnets only). */
const DEV_KEY =
  "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80" as `0x${string}`;

describe("createControlPlaneFetch", () => {
  it("returns a fetch function and wires wrapFetchWithPayment", () => {
    const f = createControlPlaneFetch({ evmPrivateKey: DEV_KEY });
    expect(typeof f).toBe("function");
    expect(wrapFetchWithPayment).toHaveBeenCalled();
  });
});
