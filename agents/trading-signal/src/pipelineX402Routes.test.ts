import { describe, expect, it } from "vitest";
import { PIPELINE_X402_ROUTE_KEYS, pipelineRouteRequiresX402 } from "./pipelineX402Routes.js";

describe("pipelineRouteRequiresX402", () => {
  it("returns false for status GET", () => {
    expect(pipelineRouteRequiresX402("GET", "/v1/pipelines/abc123/status")).toBe(false);
  });

  it("returns true for gated mutations", () => {
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines")).toBe(true);
    expect(pipelineRouteRequiresX402("post", "/v1/pipelines")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/p1/start")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/p1/stop")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/p1/pause")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/p1/resume")).toBe(true);
    expect(pipelineRouteRequiresX402("PUT", "/v1/pipelines/p1/reconfigure")).toBe(true);
    expect(pipelineRouteRequiresX402("PUT", "/v1/pipelines/p1/payment-stream")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/agents/a1/topup/x402")).toBe(true);
    expect(pipelineRouteRequiresX402("POST", "/v1/agents/a1/topup/deposit")).toBe(true);
  });

  it("does not match wrong method or path depth", () => {
    expect(pipelineRouteRequiresX402("GET", "/v1/pipelines")).toBe(false);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/p1/extra/start")).toBe(false);
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines/start")).toBe(false);
  });

  it("ignores query string on pathname", () => {
    expect(pipelineRouteRequiresX402("POST", "/v1/pipelines?x=1")).toBe(true);
  });
});

describe("PIPELINE_X402_ROUTE_KEYS", () => {
  it("has stable count aligned with Go middleware", () => {
    expect(PIPELINE_X402_ROUTE_KEYS.length).toBe(9);
  });
});
