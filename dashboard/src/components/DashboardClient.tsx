"use client";

import { useEffect, useState } from "react";
import type { Summary } from "@/lib/types";

function stateStyles(state: string) {
  switch (state) {
    case "running":
      return "bg-emerald-500/15 text-emerald-300 border-emerald-500/40";
    case "paused":
      return "bg-amber-500/15 text-amber-200 border-amber-500/35";
    case "stopped":
      return "bg-rose-500/12 text-rose-200 border-rose-500/35";
    default:
      return "bg-zinc-500/15 text-zinc-300 border-zinc-500/30";
  }
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function DashboardClient() {
  const [data, setData] = useState<Summary | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let stop = false;
    async function pull() {
      try {
        const r = await fetch("/api/backend/summary", { cache: "no-store" });
        const j = (await r.json()) as Summary & { error?: string };
        if (!r.ok) {
          throw new Error(j.error || `HTTP ${r.status}`);
        }
        if (!stop) {
          setData(j as Summary);
          setErr(null);
        }
      } catch (e) {
        if (!stop) {
          setErr(e instanceof Error ? e.message : String(e));
        }
      } finally {
        if (!stop) setLoading(false);
      }
    }
    void pull();
    const id = setInterval(() => void pull(), 3000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, []);

  const pay = data?.payments as Record<string, unknown> | undefined;
  const x402 = pay?.x402 as Record<string, unknown> | undefined;

  return (
    <div className="space-y-10">
      <header className="flex flex-col gap-4 border-b border-white/[0.08] pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="font-display text-xs uppercase tracking-[0.35em] text-[#c9a227]">
            Observability
          </p>
          <h1 className="font-display mt-2 text-4xl font-semibold tracking-tight text-zinc-50 sm:text-5xl">
            Pipeline control room
          </h1>
          <p className="mt-3 max-w-xl text-sm leading-relaxed text-zinc-400">
            Live view of Go API health, x402-derived payment signals, pipeline
            sessions, Naryo adapter traffic, and the in-process activity ring.
          </p>
        </div>
        <div className="flex flex-col items-start gap-2 text-right sm:items-end">
          <div
            className={`rounded-full border px-3 py-1 text-xs font-medium ${
              err
                ? "border-rose-500/50 bg-rose-500/10 text-rose-200"
                : "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
            }`}
          >
            {loading && !data ? "Connecting…" : err ? "Backend unreachable" : "Streaming"}
          </div>
          {data?.generatedAt && (
            <span className="font-mono text-xs text-zinc-500">
              snapshot {formatTime(data.generatedAt)}
            </span>
          )}
        </div>
      </header>

      {err && (
        <div className="rounded-xl border border-rose-500/30 bg-rose-950/30 px-4 py-3 text-sm text-rose-100">
          <span className="font-medium">Could not reach the API.</span>{" "}
          <span className="text-rose-200/80">{err}</span>
          <p className="mt-2 text-xs text-rose-200/60">
            Run the Go gateway with{" "}
            <code className="rounded bg-black/30 px-1 py-0.5 font-mono">
              go run ./cmd/api
            </code>{" "}
            and set{" "}
            <code className="rounded bg-black/30 px-1 py-0.5 font-mono">
              API_BASE_URL
            </code>{" "}
            if it is not on 127.0.0.1:8080.
          </p>
        </div>
      )}

      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="API"
          value={data?.api?.status === "ok" ? "Healthy" : "—"}
          hint="GET /healthz"
        />
        <MetricCard
          label="Pipelines"
          value={data ? String(data.pipelines.length) : "—"}
          hint="sessions in memory"
        />
        <MetricCard
          label="Est. billed (¢)"
          value={
            pay?.estimatedBilledCents != null
              ? String(pay.estimatedBilledCents)
              : "—"
          }
          hint="Σ billedSeconds × rate"
        />
        <MetricCard
          label="Active streams"
          value={
            pay?.streamsActive != null ? String(pay.streamsActive) : "—"
          }
          hint="paymentStreamActive"
        />
      </section>

      <div className="grid gap-8 lg:grid-cols-2">
        <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
          <h2 className="font-display text-lg font-medium text-zinc-100">
            Pipelines
          </h2>
          <p className="mt-1 text-xs text-zinc-500">
            States and billing counters from the orchestrator store.
          </p>
          <div className="mt-5 overflow-x-auto">
            <table className="w-full min-w-[640px] text-left text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-xs uppercase tracking-wider text-zinc-500">
                  <th className="pb-3 pr-4 font-medium">ID</th>
                  <th className="pb-3 pr-4 font-medium">Agent</th>
                  <th className="pb-3 pr-4 font-medium">State</th>
                  <th className="pb-3 pr-4 font-medium">Stream</th>
                  <th className="pb-3 pr-4 font-medium text-right">Billed s</th>
                  <th className="pb-3 font-medium">Naryo op</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {data?.pipelines?.length ? (
                  data.pipelines.map((p) => (
                    <tr key={p.id} className="text-zinc-300">
                      <td className="py-3 pr-4 font-mono text-xs text-zinc-400">
                        {p.id}
                      </td>
                      <td className="py-3 pr-4 text-zinc-400">
                        {p.agentId || "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <span
                          className={`inline-block rounded-md border px-2 py-0.5 text-xs font-medium ${stateStyles(p.state)}`}
                        >
                          {p.state}
                        </span>
                      </td>
                      <td className="py-3 pr-4">
                        {p.paymentStreamActive ? (
                          <span className="text-emerald-300/90">on</span>
                        ) : (
                          <span className="text-zinc-600">off</span>
                        )}
                      </td>
                      <td className="py-3 pr-4 text-right font-mono text-zinc-400">
                        {p.billedSeconds}
                        <span className="ml-1 text-zinc-600">
                          @ {p.rateCentsPerSecond}¢/s
                        </span>
                      </td>
                      <td className="py-3 font-mono text-xs text-zinc-500">
                        {p.lastNaryoOpId || "—"}
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td
                      colSpan={6}
                      className="py-10 text-center text-sm text-zinc-500"
                    >
                      No pipelines yet. Create one via the paid REST API.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
          <h2 className="font-display text-lg font-medium text-zinc-100">
            Naryo adapter
          </h2>
          <p className="mt-1 text-xs text-zinc-500">
            Configuration API client health (mock counts until live HTTP client).
          </p>
          <dl className="mt-5 space-y-3 font-mono text-sm">
            {data?.naryo &&
              Object.entries(data.naryo).map(([k, v]) => (
                <div
                  key={k}
                  className="flex justify-between gap-4 border-b border-white/[0.04] pb-2 last:border-0"
                >
                  <dt className="text-zinc-500">{k}</dt>
                  <dd className="text-right text-zinc-200">
                    {typeof v === "object" ? JSON.stringify(v) : String(v)}
                  </dd>
                </div>
              ))}
            {!data?.naryo && (
              <p className="text-zinc-500">No Naryo stats in payload.</p>
            )}
          </dl>
        </section>
      </div>

      <div className="grid gap-8 lg:grid-cols-2">
        <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6">
          <h2 className="font-display text-lg font-medium text-zinc-100">
            Payments (x402)
          </h2>
          <p className="mt-1 text-xs text-zinc-500">
            {typeof x402?.note === "string" ? x402.note : "Inferred summary."}
          </p>
          <ul className="mt-4 space-y-2 text-sm text-zinc-300">
            <li>
              <span className="text-zinc-500">Running pipelines:</span>{" "}
              {pay?.runningPipelines != null ? String(pay.runningPipelines) : "—"}
            </li>
            <li>
              <span className="text-zinc-500">Lifecycle / stream events (window):</span>{" "}
              {pay?.recentPaymentEvents != null
                ? String(pay.recentPaymentEvents)
                : "—"}
            </li>
          </ul>
        </section>

        <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6">
          <h2 className="font-display text-lg font-medium text-zinc-100">
            Activity
          </h2>
          <p className="mt-1 text-xs text-zinc-500">
            Last 100 lifecycle events from the in-process ring buffer.
          </p>
          <ul className="mt-4 max-h-[420px] space-y-2 overflow-y-auto pr-1 font-mono text-xs leading-relaxed">
            {data?.activity?.length ? (
              data.activity.map((a, i) => (
                <li
                  key={`${a.timestamp}-${a.type}-${i}`}
                  className="flex flex-col gap-0.5 rounded-lg border border-white/[0.04] bg-black/20 px-3 py-2"
                >
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="text-[#e8d5a3]">{a.type}</span>
                    <span className="text-zinc-600">
                      {formatTime(a.timestamp)}
                    </span>
                  </div>
                  {a.sessionId && (
                    <span className="text-zinc-500">session {a.sessionId}</span>
                  )}
                  {a.data && Object.keys(a.data).length > 0 && (
                    <pre className="mt-1 overflow-x-auto text-[11px] text-zinc-500">
                      {JSON.stringify(a.data, null, 0)}
                    </pre>
                  )}
                </li>
              ))
            ) : (
              <li className="text-zinc-500">No events recorded yet.</li>
            )}
          </ul>
        </section>
      </div>
    </div>
  );
}

function MetricCard({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint: string;
}) {
  return (
    <div className="rounded-2xl border border-white/[0.07] bg-gradient-to-br from-[#16161d] to-[#0e0e12] p-5">
      <p className="text-xs uppercase tracking-widest text-zinc-500">{label}</p>
      <p className="font-display mt-2 text-3xl font-semibold text-zinc-50">
        {value}
      </p>
      <p className="mt-1 text-xs text-zinc-600">{hint}</p>
    </div>
  );
}
