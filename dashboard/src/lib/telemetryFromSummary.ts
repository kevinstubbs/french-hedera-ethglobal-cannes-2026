import type { ActivityRow, Summary } from "@/lib/types";

export type TelemetryChartRow = {
  minuteLabel: string;
  minuteTs: number;
  pipelines: number;
  eventsPerMinute: number;
  cumulativeEvents: number;
  earnedDollars: number;
};

const DEDUPE_CAP = 8000;
export const TELEMETRY_WINDOW_MINUTES = 45;

export function floorToMinuteUtc(ms: number): number {
  return Math.floor(ms / 60000) * 60000;
}

function activityDedupeKey(a: ActivityRow): string {
  const dataKey =
    a.data && Object.keys(a.data).length > 0
      ? JSON.stringify(a.data, Object.keys(a.data).sort())
      : "";
  return `${a.timestamp}\0${a.type}\0${a.sessionId ?? ""}\0${dataKey}`;
}

export type TelemetryAccumulator = {
  dedupe: Set<string>;
  dedupeQueue: string[];
  pipelinesAtMinute: Map<number, number>;
  eventsPerMinute: Map<number, number>;
  /** Max estimated billed cents seen from any poll (monotonic). */
  earnedCentsMax: number;
  /** After a poll in minute M, cumulative cents snapshot. */
  earnedCentsAfterPollMinute: Map<number, number>;
};

export function createTelemetryAccumulator(): TelemetryAccumulator {
  return {
    dedupe: new Set(),
    dedupeQueue: [],
    pipelinesAtMinute: new Map(),
    eventsPerMinute: new Map(),
    earnedCentsMax: 0,
    earnedCentsAfterPollMinute: new Map(),
  };
}

function pushDedupe(acc: TelemetryAccumulator, key: string): boolean {
  if (acc.dedupe.has(key)) return false;
  acc.dedupe.add(key);
  acc.dedupeQueue.push(key);
  while (acc.dedupeQueue.length > DEDUPE_CAP) {
    const old = acc.dedupeQueue.shift();
    if (old) acc.dedupe.delete(old);
  }
  return true;
}

/**
 * Incorporate one summary snapshot (e.g. each poll). Mutates acc.
 */
export function ingestSummarySnapshot(
  acc: TelemetryAccumulator,
  summary: Summary,
): void {
  const pollMs = Date.parse(summary.generatedAt);
  if (Number.isNaN(pollMs)) return;
  const pollMin = floorToMinuteUtc(pollMs);

  acc.pipelinesAtMinute.set(pollMin, summary.pipelines.length);

  const pay = summary.payments as Record<string, unknown> | undefined;
  const centsRaw = pay?.estimatedBilledCents;
  const cents =
    typeof centsRaw === "number"
      ? centsRaw
      : typeof centsRaw === "string"
        ? Number(centsRaw)
        : 0;
  if (!Number.isNaN(cents)) {
    acc.earnedCentsMax = Math.max(acc.earnedCentsMax, cents);
    acc.earnedCentsAfterPollMinute.set(pollMin, acc.earnedCentsMax);
  }

  for (const row of summary.activity ?? []) {
    const t = Date.parse(row.timestamp);
    if (Number.isNaN(t)) continue;
    const em = floorToMinuteUtc(t);
    const key = activityDedupeKey(row);
    if (pushDedupe(acc, key)) {
      acc.eventsPerMinute.set(em, (acc.eventsPerMinute.get(em) ?? 0) + 1);
    }
  }
}

function forwardFill<T>(
  minutes: number[],
  source: Map<number, T>,
  fallback: T,
): T[] {
  let last = fallback;
  return minutes.map((m) => {
    if (source.has(m)) {
      last = source.get(m) as T;
    }
    return last;
  });
}

/**
 * Build chart rows for the last `windowMinutes` minute buckets (UTC).
 */
export function projectTelemetryWindow(
  acc: TelemetryAccumulator,
  windowMinutes: number = TELEMETRY_WINDOW_MINUTES,
  nowMs: number = Date.now(),
): TelemetryChartRow[] {
  const end = floorToMinuteUtc(nowMs);
  const start = end - (windowMinutes - 1) * 60000;
  const slots: number[] = [];
  for (let m = start; m <= end; m += 60000) {
    slots.push(m);
  }
  if (slots.length === 0) return [];

  const pipelines = forwardFill(slots, acc.pipelinesAtMinute, 0);

  let baseCumulative = 0;
  for (const [em, n] of acc.eventsPerMinute) {
    if (em < start) baseCumulative += n;
  }
  let cum = baseCumulative;
  const prefixCumulative: number[] = [];
  for (const m of slots) {
    cum += acc.eventsPerMinute.get(m) ?? 0;
    prefixCumulative.push(cum);
  }

  let lastEarnedCents = 0;
  for (const [pm, ec] of acc.earnedCentsAfterPollMinute) {
    if (pm < start) lastEarnedCents = Math.max(lastEarnedCents, ec);
  }
  const earnedCentsSeries: number[] = [];
  for (const m of slots) {
    if (acc.earnedCentsAfterPollMinute.has(m)) {
      lastEarnedCents = acc.earnedCentsAfterPollMinute.get(m) as number;
    }
    earnedCentsSeries.push(lastEarnedCents);
  }

  return slots.map((minuteTs, i) => ({
    minuteTs,
    minuteLabel: new Date(minuteTs).toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }),
    pipelines: pipelines[i] ?? 0,
    eventsPerMinute: acc.eventsPerMinute.get(minuteTs) ?? 0,
    cumulativeEvents: prefixCumulative[i] ?? 0,
    earnedDollars: (earnedCentsSeries[i] ?? 0) / 100,
  }));
}

export function ingestAndProject(
  acc: TelemetryAccumulator,
  summary: Summary,
  windowMinutes?: number,
  nowMs?: number,
): TelemetryChartRow[] {
  ingestSummarySnapshot(acc, summary);
  return projectTelemetryWindow(acc, windowMinutes, nowMs);
}
