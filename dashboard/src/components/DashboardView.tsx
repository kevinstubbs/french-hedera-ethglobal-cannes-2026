import { ObservabilityRealtimeChart } from "@/components/ObservabilityRealtimeChart";
import type {
  NaryoConfigurationSnapshot,
  PipelineDetailResponse,
  PipelineSessionDetail,
  Summary,
} from "@/lib/types";
import type { TelemetryChartRow } from "@/lib/telemetryFromSummary";

const HEDERA_LOCAL_NODE_README =
  "https://github.com/hiero-ledger/hiero-local-node/blob/main/README.md" as const;

export type MainTab = "observability" | "hedera";

export type DashboardViewProps = {
  tab: MainTab;
  onTabChange: (tab: MainTab) => void;
  loading: boolean;
  err: string | null;
  data: Summary | null;
  hederaExplorerUrl: string;
  /** When set, pipeline rows are interactive and a detail panel can open. */
  selectedPipelineId?: string | null;
  onSelectPipeline?: (id: string | null) => void;
  pipelineDetail?: PipelineDetailResponse | null;
  pipelineDetailLoading?: boolean;
  pipelineDetailErr?: string | null;
  /** Minute-bucket series built in the browser from summary polling. */
  telemetryRows?: TelemetryChartRow[];
  /** Live GET /observability/v1/naryo/configuration (full lists). */
  naryoConfiguration?: NaryoConfigurationSnapshot | null;
  naryoConfigurationErr?: string | null;
  /** Narrowed snapshot when a pipeline row is selected (?pipelineId=). */
  naryoConfigurationPipeline?: NaryoConfigurationSnapshot | null;
  naryoConfigurationPipelineErr?: string | null;
};

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

function JsonBlock({
  value,
  maxHeightClass = "max-h-56",
}: {
  value: unknown;
  maxHeightClass?: string;
}) {
  return (
    <pre
      className={`${maxHeightClass} overflow-auto rounded-lg border border-white/[0.06] bg-black/30 p-3 font-mono text-[11px] leading-relaxed text-zinc-400`}
    >
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function naryoSnapshotCounts(s: NaryoConfigurationSnapshot | null | undefined) {
  if (!s) return null;
  const fc = s.filtersCount;
  const bc = s.broadcastersCount;
  const cc = s.broadcasterConfigurationsCount;
  const parts: string[] = [];
  if (typeof fc === "number") parts.push(`${fc} filter(s)`);
  if (typeof bc === "number") parts.push(`${bc} broadcaster(s)`);
  if (typeof cc === "number") parts.push(`${cc} HTTP config(s)`);
  return parts.length ? parts.join(" · ") : null;
}

function naryoSnapshotHintLines(s: NaryoConfigurationSnapshot | null | undefined) {
  if (!s) return [];
  const raw = s.snapshotHints;
  if (Array.isArray(raw)) {
    return raw.filter((x): x is string => typeof x === "string");
  }
  return [];
}

function sessionMetadata(session: PipelineSessionDetail): Record<string, unknown> {
  const rest: Record<string, unknown> = { ...session };
  delete rest.config;
  delete rest.naryoFilterPlan;
  return rest;
}

function formatNaryoPlanCell(p: { naryoFilterPlan?: Record<string, unknown> }): string {
  const plan = p.naryoFilterPlan;
  if (!plan || typeof plan !== "object") return "—";
  if (plan.useALLFallback === true) return "ALL (no pf-* row)";
  const name = plan.expectedNaryoFilterName;
  if (typeof name === "string" && name) return name;
  const key = plan.key;
  return typeof key === "string" && key ? key : "—";
}

export function DashboardView({
  tab,
  onTabChange,
  loading,
  err,
  data,
  hederaExplorerUrl,
  selectedPipelineId = null,
  onSelectPipeline,
  pipelineDetail = null,
  pipelineDetailLoading = false,
  pipelineDetailErr = null,
  telemetryRows = [],
  naryoConfiguration = null,
  naryoConfigurationErr = null,
  naryoConfigurationPipeline = null,
  naryoConfigurationPipelineErr = null,
}: DashboardViewProps) {
  const pay = data?.payments as Record<string, unknown> | undefined;
  const x402 = pay?.x402 as Record<string, unknown> | undefined;

  return (
    <div className="space-y-10">
      <header className="flex flex-col gap-4 border-b border-white/[0.08] pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0 flex-1">
          <p className="font-display text-xs uppercase tracking-[0.35em] text-[#c9a227]">
            {tab === "observability" ? "Observability" : "Hedera (local node)"}
          </p>
          <h1 className="font-display mt-2 text-4xl font-semibold tracking-tight text-zinc-50 sm:text-5xl">
            {tab === "observability"
              ? "Pipeline control room"
              : "Local network explorer"}
          </h1>
          <p className="mt-3 max-w-xl text-sm leading-relaxed text-zinc-400">
            {tab === "observability" ? (
              <>
                Live view of Go API health, x402-derived payment signals,
                pipeline sessions, Naryo adapter traffic, and the in-process
                activity ring.
              </>
            ) : (
              <>
                Embedded mirror Explorer UI (default{" "}
                <code className="rounded bg-black/30 px-1 py-0.5 font-mono text-zinc-300">
                  localhost:8090
                </code>
                ). Start the network from{" "}
                <code className="rounded bg-black/30 px-1 py-0.5 font-mono text-zinc-300">
                  hedera-local-node
                </code>{" "}
                (<code className="rounded bg-black/30 px-1 py-0.5 font-mono text-zinc-300">
                  npm run start
                </code>
                ). See the{" "}
                <a
                  href={HEDERA_LOCAL_NODE_README}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-[#e8d5a3] underline decoration-[#c9a227]/40 underline-offset-2 hover:decoration-[#c9a227]"
                >
                  Hiero Local Node README
                </a>
                .
              </>
            )}
          </p>
        </div>
        <div className="flex flex-col gap-3 sm:items-end">
          <nav
            className="flex flex-wrap gap-2 rounded-xl border border-white/[0.08] bg-black/20 p-1"
            aria-label="Main views"
          >
            <button
              type="button"
              onClick={() => onTabChange("observability")}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                tab === "observability"
                  ? "bg-white/[0.12] text-zinc-50"
                  : "text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300"
              }`}
            >
              Observability
            </button>
            <button
              type="button"
              onClick={() => onTabChange("hedera")}
              className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                tab === "hedera"
                  ? "bg-white/[0.12] text-zinc-50"
                  : "text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300"
              }`}
            >
              Hedera
            </button>
          </nav>
          {tab === "observability" && (
            <div className="flex flex-col items-start gap-2 text-right sm:items-end">
              <div
                className={`rounded-full border px-3 py-1 text-xs font-medium ${
                  err
                    ? "border-rose-500/50 bg-rose-500/10 text-rose-200"
                    : "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
                }`}
              >
                {loading && !data
                  ? "Connecting…"
                  : err
                    ? "Backend unreachable"
                    : "Streaming"}
              </div>
              {data?.generatedAt && (
                <span className="font-mono text-xs text-zinc-500">
                  snapshot {formatTime(data.generatedAt)}
                </span>
              )}
            </div>
          )}
          {tab === "hedera" && (
            <a
              href={hederaExplorerUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="rounded-lg border border-white/[0.12] bg-white/[0.06] px-3 py-1.5 text-sm text-zinc-200 hover:bg-white/[0.1]"
            >
              Open Explorer in new tab
            </a>
          )}
        </div>
      </header>

      {tab === "hedera" && (
        <section className="space-y-3">
          <p className="text-xs text-zinc-500">
            If the frame stays blank, the Explorer may send{" "}
            <code className="font-mono text-zinc-400">X-Frame-Options</code> or
            your browser may block mixed content; use the link above.
          </p>
          <div className="overflow-hidden rounded-2xl border border-white/[0.08] bg-[#121218]/80 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
            <iframe
              title="Hedera local mirror explorer"
              src={hederaExplorerUrl}
              className="h-[min(78vh,900px)] w-full bg-zinc-950"
              referrerPolicy="no-referrer-when-downgrade"
            />
          </div>
        </section>
      )}

      {tab === "observability" && (
        <>
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

          <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
            <h2 className="font-display text-lg font-medium text-zinc-100">
              Realtime (this browser session)
            </h2>
            <p className="mt-1 max-w-3xl text-xs leading-relaxed text-zinc-500">
              Aggregates each summary poll (~3s) into UTC minute buckets. Activity
              rows are deduped so the ring buffer does not double-count across
              polls.
            </p>
            <div className="mt-6">
              <ObservabilityRealtimeChart data={telemetryRows} />
            </div>
          </section>

          <div className="grid gap-8 lg:grid-cols-2">
            <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
              <h2 className="font-display text-lg font-medium text-zinc-100">
                Pipelines
              </h2>
              <p className="mt-1 text-xs text-zinc-500">
                States and billing counters from the orchestrator store.
                {onSelectPipeline ? (
                  <>
                    {" "}
                    Click a row for config, session metadata, recent activity,
                    and Naryo payloads.
                  </>
                ) : null}
              </p>
              <div className="mt-5 overflow-x-auto">
                <table className="w-full min-w-[640px] text-left text-sm">
                  <thead>
                    <tr className="border-b border-white/[0.06] text-xs uppercase tracking-wider text-zinc-500">
                      <th className="pb-3 pr-4 font-medium">ID</th>
                      <th className="pb-3 pr-4 font-medium">Agent</th>
                      <th className="pb-3 pr-4 font-medium">State</th>
                      <th className="pb-3 pr-4 font-medium">Stream</th>
                      <th className="pb-3 pr-4 font-medium text-right">
                        Billed s
                      </th>
                      <th className="pb-3 pr-4 font-medium">Naryo plan</th>
                      <th className="pb-3 font-medium">Naryo op</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/[0.04]">
                    {data?.pipelines?.length ? (
                      data.pipelines.map((p) => (
                        <tr
                          key={p.id}
                          className={`text-zinc-300 ${
                            onSelectPipeline
                              ? "cursor-pointer transition-colors hover:bg-white/[0.04] focus-within:bg-white/[0.04]"
                              : ""
                          } ${
                            selectedPipelineId === p.id
                              ? "bg-white/[0.07]"
                              : ""
                          }`}
                          onClick={
                            onSelectPipeline
                              ? () => onSelectPipeline(p.id)
                              : undefined
                          }
                          onKeyDown={
                            onSelectPipeline
                              ? (e) => {
                                  if (e.key === "Enter" || e.key === " ") {
                                    e.preventDefault();
                                    onSelectPipeline(p.id);
                                  }
                                }
                              : undefined
                          }
                          tabIndex={onSelectPipeline ? 0 : undefined}
                          role={onSelectPipeline ? "button" : undefined}
                          aria-label={
                            onSelectPipeline
                              ? `Open details for pipeline ${p.id}`
                              : undefined
                          }
                        >
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
                          <td
                            className="py-3 pr-4 font-mono text-[11px] leading-snug text-zinc-500"
                            title={
                              p.naryoFilterPlan
                                ? JSON.stringify(p.naryoFilterPlan)
                                : undefined
                            }
                          >
                            {formatNaryoPlanCell(p)}
                          </td>
                          <td className="py-3 font-mono text-xs text-zinc-500">
                            {p.lastNaryoOpId || "—"}
                          </td>
                        </tr>
                      ))
                    ) : (
                      <tr>
                        <td
                          colSpan={7}
                          className="py-10 text-center text-sm text-zinc-500"
                        >
                          No pipelines yet. Create one via the paid REST API.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>

              {selectedPipelineId && onSelectPipeline ? (
                <div className="mt-6 rounded-xl border border-[#c9a227]/25 bg-black/25 p-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h3 className="font-display text-base font-medium text-zinc-100">
                        Pipeline detail
                      </h3>
                      <p className="mt-1 font-mono text-xs text-zinc-500">
                        {selectedPipelineId}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => onSelectPipeline(null)}
                      className="rounded-lg border border-white/[0.12] bg-white/[0.06] px-3 py-1.5 text-sm text-zinc-200 hover:bg-white/[0.1]"
                    >
                      Close
                    </button>
                  </div>

                  {pipelineDetailLoading && !pipelineDetail ? (
                    <p className="mt-4 text-sm text-zinc-500">Loading…</p>
                  ) : null}
                  {pipelineDetailErr ? (
                    <p className="mt-4 text-sm text-rose-200/90">
                      {pipelineDetailErr}
                    </p>
                  ) : null}

                  {pipelineDetail ? (
                    <div className="mt-5 grid gap-6 lg:grid-cols-2">
                      <div className="space-y-4">
                        <div>
                          <h4 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                            Metadata
                          </h4>
                          <p className="mt-2 text-xs text-zinc-600">
                            Session fields (billing, lifecycle, rates). Config
                            patches are shown separately.
                          </p>
                          {pipelineDetail.prepaidBalanceUnits != null ? (
                            <p className="mt-2 font-mono text-xs text-zinc-400">
                              prepaidBalanceUnits:{" "}
                              {pipelineDetail.prepaidBalanceUnits}
                            </p>
                          ) : null}
                          <div className="mt-2">
                            <JsonBlock
                              value={sessionMetadata(pipelineDetail.session)}
                            />
                          </div>
                          {pipelineDetail.session.naryoFilterPlan &&
                          Object.keys(pipelineDetail.session.naryoFilterPlan)
                            .length > 0 ? (
                            <div className="mt-4">
                              <h5 className="text-xs font-medium text-zinc-500">
                                Naryo filter plan (from config)
                              </h5>
                              <p className="mt-1 text-xs text-zinc-600">
                                What start/resume provisions in Naryo (one scoped
                                filter per pipeline). Compare{" "}
                                <code className="font-mono text-zinc-500">
                                  winningSubscriptionId
                                </code>{" "}
                                and{" "}
                                <code className="font-mono text-zinc-500">
                                  subscriptionsNotProvisionedInNaryo
                                </code>{" "}
                                to the full Config JSON — extra subscriptions
                                stay intent-only until we add multi-filter
                                support.{" "}
                                <code className="font-mono text-zinc-500">
                                  hederaNetwork
                                </code>{" "}
                                /{" "}
                                <code className="font-mono text-zinc-500">
                                  caip2
                                </code>{" "}
                                are not sent to Naryo; mirror + RPC come from
                                Naryo&apos;s YAML.
                              </p>
                              <div className="mt-2">
                                <JsonBlock
                                  value={pipelineDetail.session.naryoFilterPlan}
                                  maxHeightClass="max-h-40"
                                />
                              </div>
                            </div>
                          ) : null}
                        </div>
                        <div>
                          <h4 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                            Config
                          </h4>
                          <p className="mt-2 text-xs text-zinc-600">
                            Merged <code className="font-mono text-zinc-500">reconfigure</code>{" "}
                            payload (empty until patched).
                          </p>
                          <div className="mt-2">
                            <JsonBlock
                              value={
                                pipelineDetail.session.config &&
                                Object.keys(pipelineDetail.session.config).length > 0
                                  ? pipelineDetail.session.config
                                  : {}
                              }
                            />
                          </div>
                        </div>
                      </div>
                      <div className="space-y-6">
                        <div>
                          <h4 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                            Recent activity
                          </h4>
                          <p className="mt-2 text-xs text-zinc-600">
                            Newest first for this session only.
                          </p>
                          <ul className="mt-3 max-h-56 space-y-2 overflow-y-auto pr-1 font-mono text-xs leading-relaxed">
                            {pipelineDetail.recentActivity?.length ? (
                              pipelineDetail.recentActivity.map((a, i) => (
                                <li
                                  key={`${a.timestamp}-${a.type}-${i}`}
                                  className="rounded-lg border border-white/[0.04] bg-black/20 px-3 py-2"
                                >
                                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                                    <span className="text-[#e8d5a3]">
                                      {a.type}
                                    </span>
                                    <span className="text-zinc-600">
                                      {formatTime(a.timestamp)}
                                    </span>
                                  </div>
                                  {a.data &&
                                  Object.keys(a.data).length > 0 ? (
                                    <pre className="mt-1 overflow-x-auto text-[11px] text-zinc-500">
                                      {JSON.stringify(a.data, null, 2)}
                                    </pre>
                                  ) : null}
                                </li>
                              ))
                            ) : (
                              <li className="text-zinc-600">
                                No activity for this session in the ring buffer.
                              </li>
                            )}
                          </ul>
                        </div>
                        <div>
                          <h4 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                            Recent Naryo data
                          </h4>
                          <p className="mt-2 text-xs text-zinc-600">
                            Inbound webhook payloads (most recent first).
                          </p>
                          <ul className="mt-3 max-h-56 space-y-2 overflow-y-auto pr-1 font-mono text-xs">
                            {pipelineDetail.recentNaryoEvents?.length ? (
                              [...pipelineDetail.recentNaryoEvents]
                                .reverse()
                                .map((ev) => (
                                  <li
                                    key={ev.eventId}
                                    className="rounded-lg border border-white/[0.04] bg-black/20 px-3 py-2"
                                  >
                                    <div className="flex flex-wrap justify-between gap-2 text-zinc-400">
                                      <span>{ev.eventId}</span>
                                      <span className="text-zinc-600">
                                        {formatTime(ev.receivedAt)}
                                      </span>
                                    </div>
                                    {ev.payload &&
                                    Object.keys(ev.payload).length > 0 ? (
                                      <pre className="mt-1 overflow-x-auto text-[11px] text-zinc-500">
                                        {JSON.stringify(ev.payload, null, 2)}
                                      </pre>
                                    ) : null}
                                  </li>
                                ))
                            ) : (
                              <li className="text-zinc-600">
                                No Naryo events stored for this session.
                              </li>
                            )}
                          </ul>
                        </div>
                      </div>
                      {selectedPipelineId ? (
                        <div className="col-span-full mt-2 border-t border-white/[0.06] pt-6">
                          <h4 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                            Naryo Configuration (this pipeline)
                          </h4>
                          <p className="mt-2 text-xs text-zinc-600">
                            Same endpoint with{" "}
                            <code className="font-mono text-zinc-500">
                              ?pipelineId=
                            </code>{" "}
                            — filters named{" "}
                            <code className="font-mono text-zinc-500">
                              pf-&#123;sessionId&#125;-…
                            </code>{" "}
                            and broadcasters whose destination path includes this
                            session id.
                          </p>
                          {naryoConfigurationPipelineErr ? (
                            <p className="mt-3 text-sm text-rose-200/90">
                              {naryoConfigurationPipelineErr}
                            </p>
                          ) : null}
                          {naryoConfigurationPipeline ? (
                            <div className="mt-3 space-y-2">
                              {naryoSnapshotCounts(naryoConfigurationPipeline) ? (
                                <p className="font-mono text-xs text-zinc-500">
                                  {naryoSnapshotCounts(naryoConfigurationPipeline)}{" "}
                                  (after narrow)
                                </p>
                              ) : null}
                              {naryoSnapshotHintLines(naryoConfigurationPipeline)
                                .length > 0 ? (
                                <ul className="list-inside list-disc space-y-1 rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-100/95">
                                  {naryoSnapshotHintLines(
                                    naryoConfigurationPipeline,
                                  ).map((line, i) => (
                                    <li key={i}>{line}</li>
                                  ))}
                                </ul>
                              ) : null}
                              <JsonBlock
                                value={naryoConfigurationPipeline}
                                maxHeightClass="max-h-72"
                              />
                            </div>
                          ) : !naryoConfigurationPipelineErr ? (
                            <p className="mt-3 text-sm text-zinc-500">
                              Loading narrowed snapshot…
                            </p>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ) : null}
            </section>

            <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
              <h2 className="font-display text-lg font-medium text-zinc-100">
                Naryo adapter
              </h2>
              <p className="mt-1 text-xs text-zinc-500">
                Configuration API client health from the live HTTP adapter
                (ensureCalls, pauseCalls, …).
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

          <section className="rounded-2xl border border-white/[0.07] bg-[#121218]/80 p-6 shadow-[0_0_0_1px_rgba(255,255,255,0.03)_inset]">
            <h2 className="font-display text-lg font-medium text-zinc-100">
              Naryo Configuration API
            </h2>
            <p className="mt-1 max-w-3xl text-xs leading-relaxed text-zinc-500">
              Live read from Naryo (
              <code className="font-mono text-zinc-400">GET /api/v1/filters</code>,{" "}
              <code className="font-mono text-zinc-400">/broadcasters</code>,{" "}
              <code className="font-mono text-zinc-400">
                /broadcaster-configurations
              </code>
              ), plus orchestrator in-memory session state (
              <code className="font-mono text-zinc-500">mode: http</code>).{" "}
              <span className="text-zinc-400">
                ALL-target pipelines have broadcasters but often no{" "}
                <code className="font-mono text-zinc-500">pf-*</code> filter
                rows
              </span>
              ; check the pipeline row &quot;Naryo plan&quot; and the detail
              panel. When a pipeline is selected, this section can narrow to that
              session (
              <code className="font-mono text-zinc-500">pf-&#123;id&#125;-*</code>{" "}
              names and matching HTTP paths).
            </p>
            {naryoConfigurationErr ? (
              <p className="mt-4 text-sm text-rose-200/90">
                {naryoConfigurationErr}
              </p>
            ) : null}
            {naryoConfiguration ? (
              <div className="mt-4 space-y-3">
                <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 font-mono text-xs text-zinc-400">
                  <span>
                    mode:{" "}
                    <span className="text-[#e8d5a3]">
                      {String(naryoConfiguration.mode ?? "—")}
                    </span>
                  </span>
                  {typeof naryoConfiguration.configurationApiBaseURL ===
                  "string" ? (
                    <span className="text-zinc-500">
                      base: {naryoConfiguration.configurationApiBaseURL}
                    </span>
                  ) : null}
                  {naryoSnapshotCounts(naryoConfiguration) ? (
                    <span className="text-zinc-500">
                      {naryoSnapshotCounts(naryoConfiguration)}
                    </span>
                  ) : null}
                </div>
                {typeof naryoConfiguration.generatedAt === "string" ? (
                  <p className="text-xs text-zinc-600">
                    snapshot {formatTime(naryoConfiguration.generatedAt)}
                  </p>
                ) : null}
                {naryoSnapshotHintLines(naryoConfiguration).length > 0 ? (
                  <ul className="list-inside list-disc space-y-1 rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-100/95">
                    {naryoSnapshotHintLines(naryoConfiguration).map((line, i) => (
                      <li key={i}>{line}</li>
                    ))}
                  </ul>
                ) : null}
                <JsonBlock value={naryoConfiguration} maxHeightClass="max-h-[min(28rem,55vh)]" />
              </div>
            ) : !naryoConfigurationErr ? (
              <p className="mt-4 text-sm text-zinc-500">
                No configuration snapshot yet (waiting for first poll).
              </p>
            ) : null}
          </section>

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
                  {pay?.runningPipelines != null
                    ? String(pay.runningPipelines)
                    : "—"}
                </li>
                <li>
                  <span className="text-zinc-500">
                    Lifecycle / stream events (window):
                  </span>{" "}
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
                        <span className="text-zinc-500">
                          session {a.sessionId}
                        </span>
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
        </>
      )}
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
