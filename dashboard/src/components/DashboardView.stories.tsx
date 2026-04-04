import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useState } from "react";
import {
  DashboardView,
  type DashboardViewProps,
  type MainTab,
} from "@/components/DashboardView";
import { mockSummaryEmpty, mockSummaryLoaded } from "@/lib/mockSummary";

const DEFAULT_EXPLORER = "about:blank";

function StatefulView(
  props: Omit<DashboardViewProps, "tab" | "onTabChange"> & {
    initialTab?: MainTab;
  },
) {
  const { initialTab = "observability", ...rest } = props;
  const [tab, setTab] = useState<MainTab>(initialTab);
  return (
    <DashboardView
      tab={tab}
      onTabChange={setTab}
      {...rest}
      hederaExplorerUrl={rest.hederaExplorerUrl ?? DEFAULT_EXPLORER}
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
