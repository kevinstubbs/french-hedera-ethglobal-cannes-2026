"use client";

import type { TelemetryChartRow } from "@/lib/telemetryFromSummary";
import {
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

const tickStyle = { fill: "#71717a", fontSize: 10 };
const gridStroke = "rgba(255,255,255,0.06)";
const tooltipBox = {
  backgroundColor: "rgba(18,18,24,0.96)",
  border: "1px solid rgba(255,255,255,0.08)",
  borderRadius: "8px",
  fontSize: "12px",
  color: "#e4e4e7",
};

function ChartTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ dataKey?: string; value?: number; color?: string }>;
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="px-3 py-2 shadow-lg" style={tooltipBox}>
      <p className="mb-1 font-mono text-xs text-zinc-500">{label}</p>
      <ul className="space-y-0.5 font-mono text-[11px]">
        {payload.map((p) => (
          <li key={String(p.dataKey)} className="flex items-center gap-2">
            <span
              className="h-2 w-2 shrink-0 rounded-full"
              style={{ backgroundColor: p.color }}
              aria-hidden
            />
            <span className="text-zinc-400">{p.dataKey}:</span>
            <span className="text-zinc-100">
              {p.dataKey === "earnedDollars"
                ? `$${Number(p.value).toFixed(2)}`
                : String(p.value)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function ObservabilityRealtimeChart({
  data,
}: {
  data: TelemetryChartRow[];
}) {
  if (data.length === 0) {
    return (
      <div className="flex h-[min(280px,40vh)] items-center justify-center rounded-xl border border-white/[0.07] bg-black/20 text-sm text-zinc-500">
        Waiting for poll samples…
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div>
        <h3 className="mb-1 text-xs font-medium uppercase tracking-wider text-zinc-500">
          Pipelines & events / minute
        </h3>
        <p className="mb-3 text-[11px] text-zinc-600">
          Pipeline count uses the last sample in each minute (step). Event rate
          uses deduped activity timestamps (spiky).
        </p>
        <ResponsiveContainer width="100%" height={240}>
          <ComposedChart
            data={data}
            margin={{ top: 8, right: 8, left: 0, bottom: 0 }}
          >
            <CartesianGrid stroke={gridStroke} vertical={false} />
            <XAxis
              dataKey="minuteLabel"
              tick={tickStyle}
              tickLine={false}
              axisLine={{ stroke: gridStroke }}
              interval="preserveStartEnd"
            />
            <YAxis
              yAxisId="left"
              tick={tickStyle}
              tickLine={false}
              axisLine={{ stroke: gridStroke }}
              allowDecimals={false}
              width={36}
            />
            <Tooltip content={<ChartTooltip />} />
            <Legend
              wrapperStyle={{ fontSize: "11px", color: "#a1a1aa" }}
              formatter={(value) => (
                <span className="text-zinc-400">{value}</span>
              )}
            />
            <Line
              yAxisId="left"
              type="stepAfter"
              dataKey="pipelines"
              name="pipelines"
              stroke="#c9a227"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
            <Line
              yAxisId="left"
              type="monotone"
              dataKey="eventsPerMinute"
              name="events/min"
              stroke="#34d399"
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      <div>
        <h3 className="mb-1 text-xs font-medium uppercase tracking-wider text-zinc-500">
          Cumulative events & est. billed
        </h3>
        <p className="mb-3 text-[11px] text-zinc-600">
          Events: running total of unique activity rows seen. Dollars: from{" "}
          <code className="font-mono text-zinc-500">estimatedBilledCents</code>{" "}
          (monotonic in this session).
        </p>
        <ResponsiveContainer width="100%" height={240}>
          <ComposedChart
            data={data}
            margin={{ top: 8, right: 12, left: 0, bottom: 0 }}
          >
            <CartesianGrid stroke={gridStroke} vertical={false} />
            <XAxis
              dataKey="minuteLabel"
              tick={tickStyle}
              tickLine={false}
              axisLine={{ stroke: gridStroke }}
              interval="preserveStartEnd"
            />
            <YAxis
              yAxisId="ev"
              tick={tickStyle}
              tickLine={false}
              axisLine={{ stroke: gridStroke }}
              allowDecimals={false}
              width={40}
            />
            <YAxis
              yAxisId="usd"
              orientation="right"
              tick={tickStyle}
              tickLine={false}
              axisLine={{ stroke: gridStroke }}
              tickFormatter={(v) => `$${v}`}
              width={44}
            />
            <Tooltip content={<ChartTooltip />} />
            <Legend
              wrapperStyle={{ fontSize: "11px", color: "#a1a1aa" }}
              formatter={(value) => (
                <span className="text-zinc-400">{value}</span>
              )}
            />
            <Line
              yAxisId="ev"
              type="monotone"
              dataKey="cumulativeEvents"
              name="events (cumulative)"
              stroke="#a78bfa"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
            <Line
              yAxisId="usd"
              type="stepAfter"
              dataKey="earnedDollars"
              name="earned ($)"
              stroke="#e8d5a3"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
