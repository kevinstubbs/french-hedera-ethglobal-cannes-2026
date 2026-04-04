import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useMemo, useState } from "react";
import {
  DashboardView,
  type DashboardViewProps,
  type MainTab,
} from "@/components/DashboardView";
import type { PipelineDetailResponse, Summary } from "@/lib/types";
import {
  mockPipelineDetailSess8f2a1c,
  mockSummaryEmpty,
  mockSummaryLoaded,
} from "@/lib/mockSummary";

const DEFAULT_EXPLORER = "about:blank";

function StatefulView(
  props: Omit<
    DashboardViewProps,
    | "tab"
    | "onTabChange"
    | "selectedPipelineId"
    | "onSelectPipeline"
    | "pipelineDetail"
    | "pipelineDetailLoading"
    | "pipelineDetailErr"
  > & {
    initialTab?: MainTab;
    /** Pre-select a pipeline row and show mock detail (Storybook). */
    initialSelectedPipelineId?: string | null;
  },
) {
  const {
    initialTab = "observability",
    initialSelectedPipelineId = null,
    ...rest
  } = props;
  const [tab, setTab] = useState<MainTab>(initialTab);
  const [selectedId, setSelectedId] = useState<string | null>(
    initialSelectedPipelineId,
  );
  const data = rest.data as Summary | null;
  const pipelineDetail = useMemo((): PipelineDetailResponse | null => {
    if (!selectedId) return null;
    if (selectedId === "sess-8f2a1c") return mockPipelineDetailSess8f2a1c;
    const row = data?.pipelines?.find((p) => p.id === selectedId);
    if (!row) return null;
    return {
      session: {
        id: row.id,
        agentId: row.agentId,
        state: row.state,
        paymentStreamActive: row.paymentStreamActive,
        billedSeconds: row.billedSeconds,
        rateCentsPerSecond: row.rateCentsPerSecond,
        lastNaryoOpId: row.lastNaryoOpId,
        config: row.config,
      },
      recentActivity: [],
      recentNaryoEvents: [],
    };
  }, [selectedId, data]);
  return (
    <DashboardView
      tab={tab}
      onTabChange={setTab}
      {...rest}
      hederaExplorerUrl={rest.hederaExplorerUrl ?? DEFAULT_EXPLORER}
      selectedPipelineId={selectedId}
      onSelectPipeline={setSelectedId}
      pipelineDetail={pipelineDetail}
      pipelineDetailLoading={false}
      pipelineDetailErr={null}
    />
  );
}

const meta = {
  title: "Dashboard/Observability",
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta;

export default meta;

type Story = StoryObj<typeof meta>;

export const Loading: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        loading
        err={null}
        data={null}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};

export const ApiUnreachable: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        loading={false}
        err="fetch failed"
        data={null}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};

export const EmptyBackend: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        loading={false}
        err={null}
        data={mockSummaryEmpty}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};

export const WithSampleData: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        loading={false}
        err={null}
        data={mockSummaryLoaded}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};

export const PipelineDetailOpen: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        initialSelectedPipelineId="sess-8f2a1c"
        loading={false}
        err={null}
        data={mockSummaryLoaded}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};

export const HederaTab: Story = {
  render: () => (
    <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
      <StatefulView
        initialTab="hedera"
        loading={false}
        err={null}
        data={mockSummaryLoaded}
        hederaExplorerUrl={DEFAULT_EXPLORER}
      />
    </div>
  ),
};
