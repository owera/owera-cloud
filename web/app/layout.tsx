import type { Metadata } from "next";
import { Fraunces, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";

// Fraunces — variable serif with optical-size axis. Used for the editorial
// display moments on /compose. Distinctive on purpose: this is what makes
// the front door stop looking like every other dev tool.
const fraunces = Fraunces({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-display-src",
  axes: ["opsz", "SOFT"],
});

// JetBrains Mono — replaces the system mono stack with a real file.
const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-mono-src",
});

export const metadata: Metadata = {
  title: {
    default: "Owera Agentic",
    template: "%s · Owera Agentic",
  },
  description: "Agentic work as a managed service.",
  // Owera is a Brazilian company; UI is English-only for now.
  // Localization (pt-BR) is a V2+ effort.
  icons: { icon: "/favicon.svg" },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      className={`dark ${fraunces.variable} ${jetbrainsMono.variable}`}
    >
      <body className="min-h-screen antialiased">{children}</body>
    </html>
  );
}
