import { DashboardClient } from "@/components/DashboardClient";

export default function Home() {
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
      <div className="relative mx-auto max-w-7xl px-4 py-12 sm:px-6 lg:px-10 lg:py-16">
        <DashboardClient />
      </div>
    </div>
  );
}
