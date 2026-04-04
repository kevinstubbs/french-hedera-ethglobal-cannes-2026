import type { Preview } from "@storybook/nextjs-vite";
import { Fraunces, Source_Sans_3 } from "next/font/google";
import "../src/app/globals.css";

const display = Fraunces({
  subsets: ["latin"],
  variable: "--font-display",
});

const sans = Source_Sans_3({
  subsets: ["latin"],
  variable: "--font-sans",
});

const preview: Preview = {
  decorators: [
    (Story) => (
      <div
        className={`${display.variable} ${sans.variable} min-h-screen bg-[#0a0a0f] font-sans text-zinc-200 antialiased`}
      >
        <Story />
      </div>
    ),
  ],
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
  },
};

export default preview;
