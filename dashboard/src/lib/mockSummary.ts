import type { Summary } from "@/lib/types";

/** Fixed timestamp for stable Storybook screenshots */
const SNAPSHOT_AT = "2026-04-04T15:30:00.000Z";

export const mockSummaryEmpty: Summary = {
  generatedAt: SNAPSHOT_AT,
  api: { status: "ok" },
  pipelines: [],
  activity: [],
  naryo: {},
  payments: {
    x402: { note: "No payment traffic yet (mock empty state)." },
  },
};

export const mockSummaryLoaded: Summary = {
  generatedAt: SNAPSHOT_AT,
  api: { status: "ok" },
  pipelines: [
    {
      id: "sess-8f2a1c",
      agentId: "agent-demo",
      state: "running",
      paymentStreamActive: true,
      billedSeconds: 142,
      rateCentsPerSecond: 2,
      lastNaryoOpId: "naryo-op-003",
    },
    {
      id: "sess-441b90",
      agentId: "agent-demo",
      state: "paused",
      paymentStreamActive: false,
      billedSeconds: 60,
      rateCentsPerSecond: 2,
    },
    {
      id: "sess-eedd12",
      state: "stopped",
      paymentStreamActive: false,
      billedSeconds: 900,
      rateCentsPerSecond: 1,
    },
  ],
  activity: [
    {
      timestamp: "2026-04-04T15:29:55.000Z",
      type: "pipeline.started",
      sessionId: "sess-8f2a1c",
      data: { agentId: "agent-demo" },
    },
    {
      timestamp: "2026-04-04T15:29:50.000Z",
      type: "payment.stream_active",
      sessionId: "sess-8f2a1c",
    },
    {
      timestamp: "2026-04-04T15:29:40.000Z",
      type: "naryo.request",
      data: { op: "configure" },
    },
  ],
  naryo: {
    healthy: true,
    lastStatus: 200,
    requestsTotal: 48,
  },
  payments: {
    estimatedBilledCents: 344,
    streamsActive: 1,
    runningPipelines: 1,
    recentPaymentEvents: 12,
    x402: { note: "Mock x402 summary for Storybook." },
  },
};
