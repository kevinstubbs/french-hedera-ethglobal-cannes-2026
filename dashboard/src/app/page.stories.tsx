import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useState } from "react";
import { DashboardView, type MainTab } from "@/components/DashboardView";
import { mockSummaryLoaded } from "@/lib/mockSummary";
import type { Summary } from "@/lib/types";

function HomePagePreview(props: {
  loading: boolean;
  err: string | null;
  data: Summary | null;
}) {
  const [tab, setTab] = useState<MainTab>("observability");
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

export const Loading: Story = {
  render: () => (
    <HomePagePreview loading err={null} data={null} />
  ),
};
