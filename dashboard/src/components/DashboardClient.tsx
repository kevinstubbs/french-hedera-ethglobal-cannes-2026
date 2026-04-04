"use client";

import { useEffect, useMemo, useState } from "react";
import { DashboardView, type MainTab } from "@/components/DashboardView";
import type { Summary } from "@/lib/types";

export function DashboardClient() {
  const [tab, setTab] = useState<MainTab>("observability");
  const [data, setData] = useState<Summary | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const hederaExplorerUrl = useMemo(
    () =>
      (process.env.NEXT_PUBLIC_HEDERA_EXPLORER_URL || "http://localhost:8080").replace(
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

  return (
    <DashboardView
      tab={tab}
      onTabChange={setTab}
      loading={loading}
      err={err}
      data={data}
      hederaExplorerUrl={hederaExplorerUrl}
    />
  );
}
