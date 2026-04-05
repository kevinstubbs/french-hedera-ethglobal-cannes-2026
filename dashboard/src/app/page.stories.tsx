import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useMemo, useState } from "react";
import { DashboardView, type MainTab } from "@/components/DashboardView";
import {
  mockNaryoConfigurationFull,
  mockNaryoConfigurationPipeline,
  mockPipelineDetailSess8f2a1c,
  mockSummaryLoaded,
} from "@/lib/mockSummary";
import { mockTelemetryChartRows } from "@/lib/mockTelemetry";
import type { PipelineDetailResponse, Summary } from "@/lib/types";
import type { TelemetryChartRow } from "@/lib/telemetryFromSummary";

function HomePagePreview(props: {
  loading: boolean;
  err: string | null;
  data: Summary | null;
  initialSelectedPipelineId?: string | null;
  telemetryRows?: TelemetryChartRow[];
}) {
  const [tab, setTab] = useState<MainTab>("observability");
  const [selectedId, setSelectedId] = useState<string | null>(
    props.initialSelectedPipelineId ?? null,
  );
  const pipelineDetail = useMemo((): PipelineDetailResponse | null => {
    if (!selectedId) return null;
    if (selectedId === "sess-8f2a1c") return mockPipelineDetailSess8f2a1c;
    const row = props.data?.pipelines?.find((p) => p.id === selectedId);
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
  }, [selectedId, props.data]);
  const telemetryRows =
    props.telemetryRows ??
    (props.data ? mockTelemetryChartRows : []);
  const naryoPipe =
    selectedId === "sess-8f2a1c"
      ? mockNaryoConfigurationPipeline
      : selectedId
        ? {
            mode: "http",
            generatedAt: mockSummaryLoaded.generatedAt,
            pipelineId: selectedId,
            filterNamePrefix: `pf-${selectedId}-`,
            filters: [],
            broadcasters: [],
            broadcasterConfigurations: [],
          }
        : null;
  return (
    <div className="relative min-h-screen overflow-hidden">
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.35]"
        aria-hidden
        style={{
          backgroundImage: `
            radial-gradient(ellipse 80% 50% at 50% -20%, rgba(201, 162, 39, 0.12), transparent),
            radial-gradient(ellipse 60% 40% at 100% 50%, rgba(120, 80, 200, 0.06), transparent)
          `,
        }}
      />
      <div className="relative mx-auto max-w-[min(100%,1680px)] px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
        <DashboardView
          tab={tab}
          onTabChange={setTab}
          loading={props.loading}
          err={props.err}
          data={props.data}
          hederaExplorerUrl="about:blank"
          selectedPipelineId={selectedId}
          onSelectPipeline={setSelectedId}
          pipelineDetail={pipelineDetail}
          pipelineDetailLoading={false}
          pipelineDetailErr={null}
          telemetryRows={telemetryRows}
          naryoConfiguration={props.data ? mockNaryoConfigurationFull : null}
          naryoConfigurationErr={null}
          naryoConfigurationPipeline={props.data ? naryoPipe : null}
          naryoConfigurationPipelineErr={null}
        />
      </div>
    </div>
  );
}

const meta = {
  title: "Dashboard/Home page",
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta;

export default meta;

type Story = StoryObj<typeof meta>;

export const DefaultWithMockData: Story = {
  render: () => (
    <HomePagePreview
      loading={false}
      err={null}
      data={mockSummaryLoaded}
    />
  ),
};

export const PipelineSelected: Story = {
  render: () => (
    <HomePagePreview
      initialSelectedPipelineId="sess-8f2a1c"
      loading={false}
      err={null}
      data={mockSummaryLoaded}
    />
  ),
};

export const Loading: Story = {
  render: () => (
    <HomePagePreview loading err={null} data={null} />
  ),
};
