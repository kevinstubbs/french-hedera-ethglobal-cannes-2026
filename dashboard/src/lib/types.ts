export type PipelineRow = {
  id: string;
  agentId?: string;
  state: string;
  paymentStreamActive: boolean;
  billedSeconds: number;
  rateCentsPerSecond: number;
  lastNaryoOpId?: string;
  config?: Record<string, unknown>;
};

/** Full session from observability / status (includes merged reconfigure patch). */
export type PipelineSessionDetail = {
  id: string;
  agentId?: string;
  state: string;
  paymentStreamActive: boolean;
  billedSeconds: number;
  rateCentsPerSecond: number;
  lastNaryoOpId?: string;
  config?: Record<string, unknown>;
  rateUnitsPerMinute?: number;
  chargedUnits?: number;
  lastPaidMinute?: number;
  summaryWindowMinutes?: number;
  autoPausedForFunds?: boolean;
};

export type NaryoInboundEventRow = {
  eventId: string;
  payload?: Record<string, unknown>;
  receivedAt: string;
};

export type PipelineDetailResponse = {
  session: PipelineSessionDetail;
  recentActivity: ActivityRow[];
  recentNaryoEvents?: NaryoInboundEventRow[];
  prepaidBalanceUnits?: number;
};

export type ActivityRow = {
  timestamp: string;
  type: string;
  sessionId?: string;
  data?: Record<string, unknown>;
};

export type Summary = {
  generatedAt: string;
  api: { status: string };
  pipelines: PipelineRow[];
  activity: ActivityRow[];
  naryo: Record<string, unknown>;
  payments: Record<string, unknown>;
};
