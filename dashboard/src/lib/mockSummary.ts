import type { PipelineDetailResponse, Summary } from "@/lib/types";

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
      config: { template: "signal-v2", region: "eu-west" },
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

/** Detail payload when a row is selected (matches GET /observability/v1/pipelines/{id}). */
export const mockPipelineDetailSess8f2a1c: PipelineDetailResponse = {
  session: {
    id: "sess-8f2a1c",
    agentId: "agent-demo",
    state: "running",
    paymentStreamActive: true,
    billedSeconds: 142,
    rateCentsPerSecond: 2,
    lastNaryoOpId: "naryo-op-003",
    config: { template: "signal-v2", region: "eu-west" },
    rateUnitsPerMinute: 60,
    chargedUnits: 120,
    summaryWindowMinutes: 10,
    autoPausedForFunds: false,
  },
  prepaidBalanceUnits: 5000,
  recentActivity: [
    {
      timestamp: "2026-04-04T15:29:55.000Z",
      type: "pipeline_started",
      sessionId: "sess-8f2a1c",
      data: { naryoOpId: "naryo-op-003", agentId: "agent-demo" },
    },
    {
      timestamp: "2026-04-04T15:29:40.000Z",
      type: "payment_stream",
      sessionId: "sess-8f2a1c",
      data: { active: true },
    },
    {
      timestamp: "2026-04-04T15:29:20.000Z",
      type: "pipeline_created",
      sessionId: "sess-8f2a1c",
      data: { agentId: "agent-demo" },
    },
  ],
  recentNaryoEvents: [
    {
      eventId: "evt-99",
      payload: { kind: "tick", value: 1 },
      receivedAt: "2026-04-04T15:29:58.000Z",
    },
    {
      eventId: "evt-98",
      payload: { kind: "handshake" },
      receivedAt: "2026-04-04T15:29:52.000Z",
    },
  ],
};
