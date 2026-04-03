export type PipelineRow = {
  id: string;
  agentId?: string;
  state: string;
  paymentStreamActive: boolean;
  billedSeconds: number;
  rateCentsPerSecond: number;
  lastNaryoOpId?: string;
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
