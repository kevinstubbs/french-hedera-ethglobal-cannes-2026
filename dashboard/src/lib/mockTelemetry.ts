import type { TelemetryChartRow } from "@/lib/telemetryFromSummary";
import { floorToMinuteUtc } from "@/lib/telemetryFromSummary";

/** Deterministic series for Storybook (local time labels depend on TZ). */
export const mockTelemetryChartRows: TelemetryChartRow[] = (() => {
  const end = floorToMinuteUtc(Date.parse("2026-04-04T16:00:00.000Z"));
  const out: TelemetryChartRow[] = [];
  let cum = 0;
  let earnedCents = 0;
  for (let i = 35; i >= 0; i--) {
    const minuteTs = end - i * 60000;
    const stepPhase = 35 - i;
    const pipelines =
      stepPhase < 8 ? 0 : stepPhase < 18 ? 1 : stepPhase < 28 ? 2 : 3;
    const spm =
      stepPhase % 7 === 0
        ? 14
        : stepPhase % 5 === 0
          ? 8
          : stepPhase % 4 === 0
            ? 4
            : stepPhase % 3;
    cum += spm;
    earnedCents += spm * 2;
    out.push({
      minuteTs,
      minuteLabel: new Date(minuteTs).toLocaleTimeString(undefined, {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }),
      pipelines,
      eventsPerMinute: spm,
      cumulativeEvents: cum,
      earnedDollars: earnedCents / 100,
    });
  }
  return out;
})();
