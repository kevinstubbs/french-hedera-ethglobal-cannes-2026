"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { DashboardView, type MainTab } from "@/components/DashboardView";
import type {
  NaryoConfigurationSnapshot,
  PipelineDetailResponse,
  Summary,
} from "@/lib/types";
import {
  createTelemetryAccumulator,
  ingestAndProject,
  type TelemetryChartRow,
} from "@/lib/telemetryFromSummary";

export function DashboardClient() {
  const [tab, setTab] = useState<MainTab>("observability");
  const [data, setData] = useState<Summary | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedPipelineId, setSelectedPipelineId] = useState<string | null>(
    null,
  );
  const [pipelineDetail, setPipelineDetail] =
    useState<PipelineDetailResponse | null>(null);
  const [pipelineDetailLoading, setPipelineDetailLoading] = useState(false);
  const [pipelineDetailErr, setPipelineDetailErr] = useState<string | null>(
    null,
  );
  const telemetryAcc = useRef(createTelemetryAccumulator());
  const [telemetryRows, setTelemetryRows] = useState<TelemetryChartRow[]>([]);
  const [naryoConfig, setNaryoConfig] =
    useState<NaryoConfigurationSnapshot | null>(null);
  const [naryoConfigErr, setNaryoConfigErr] = useState<string | null>(null);
  const [naryoConfigPipe, setNaryoConfigPipe] =
    useState<NaryoConfigurationSnapshot | null>(null);
  const [naryoConfigPipeErr, setNaryoConfigPipeErr] = useState<string | null>(
    null,
  );

  const hederaExplorerUrl = useMemo(
    () =>
      (process.env.NEXT_PUBLIC_HEDERA_EXPLORER_URL || "http://localhost:8090").replace(
        /\/$/,
        "",
      ),
    [],
  );

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
          const s = j as Summary;
          setData(s);
          setTelemetryRows(ingestAndProject(telemetryAcc.current, s));
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

  useEffect(() => {
    let stop = false;
    async function pullNaryo() {
      try {
        const r = await fetch("/api/backend/naryo-configuration", {
          cache: "no-store",
        });
        const j = (await r.json()) as NaryoConfigurationSnapshot & {
          error?: string;
        };
        if (!r.ok) {
          throw new Error(j.error || `HTTP ${r.status}`);
        }
        if (!stop) {
          setNaryoConfig(j as NaryoConfigurationSnapshot);
          setNaryoConfigErr(null);
        }
      } catch (e) {
        if (!stop) {
          setNaryoConfigErr(e instanceof Error ? e.message : String(e));
        }
      }
    }
    void pullNaryo();
    const nid = setInterval(() => void pullNaryo(), 3000);
    return () => {
      stop = true;
      clearInterval(nid);
    };
  }, []);

  useEffect(() => {
    if (!selectedPipelineId) {
      setNaryoConfigPipe(null);
      setNaryoConfigPipeErr(null);
      return;
    }
    const pipelineId = selectedPipelineId;
    let stop = false;
    async function pullNaryoPipe() {
      try {
        const r = await fetch(
          `/api/backend/naryo-configuration?pipelineId=${encodeURIComponent(pipelineId)}`,
          { cache: "no-store" },
        );
        const j = (await r.json()) as NaryoConfigurationSnapshot & {
          error?: string;
        };
        if (!r.ok) {
          throw new Error(j.error || `HTTP ${r.status}`);
        }
        if (!stop) {
          setNaryoConfigPipe(j as NaryoConfigurationSnapshot);
          setNaryoConfigPipeErr(null);
        }
      } catch (e) {
        if (!stop) {
          setNaryoConfigPipeErr(
            e instanceof Error ? e.message : String(e),
          );
        }
      }
    }
    void pullNaryoPipe();
    const pid = setInterval(() => void pullNaryoPipe(), 3000);
    return () => {
      stop = true;
      clearInterval(pid);
    };
  }, [selectedPipelineId]);

  useEffect(() => {
    if (!selectedPipelineId) {
      setPipelineDetail(null);
      setPipelineDetailErr(null);
      setPipelineDetailLoading(false);
      return;
    }
    const pipelineId = selectedPipelineId;
    setPipelineDetail(null);
    setPipelineDetailErr(null);
    let stop = false;
    let initial = true;
    setPipelineDetailLoading(true);
    async function pullDetail() {
      try {
        const r = await fetch(
          `/api/backend/pipelines/${encodeURIComponent(pipelineId)}`,
          { cache: "no-store" },
        );
        const j = (await r.json()) as PipelineDetailResponse & {
          error?: string;
        };
        if (!r.ok) {
          throw new Error(j.error || `HTTP ${r.status}`);
        }
        if (!stop) {
          setPipelineDetail(j as PipelineDetailResponse);
          setPipelineDetailErr(null);
        }
      } catch (e) {
        if (!stop) {
          setPipelineDetailErr(
            e instanceof Error ? e.message : String(e),
          );
        }
      } finally {
        if (!stop && initial) {
          initial = false;
          setPipelineDetailLoading(false);
        }
      }
    }
    void pullDetail();
    const detailPoll = setInterval(() => void pullDetail(), 3000);
    return () => {
      stop = true;
      clearInterval(detailPoll);
    };
  }, [selectedPipelineId]);

  return (
    <DashboardView
      tab={tab}
      onTabChange={setTab}
      loading={loading}
      err={err}
      data={data}
      hederaExplorerUrl={hederaExplorerUrl}
      selectedPipelineId={selectedPipelineId}
      onSelectPipeline={setSelectedPipelineId}
      pipelineDetail={pipelineDetail}
      pipelineDetailLoading={pipelineDetailLoading}
      pipelineDetailErr={pipelineDetailErr}
      telemetryRows={telemetryRows}
      naryoConfiguration={naryoConfig}
      naryoConfigurationErr={naryoConfigErr}
      naryoConfigurationPipeline={naryoConfigPipe}
      naryoConfigurationPipelineErr={naryoConfigPipeErr}
    />
  );
}
