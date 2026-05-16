import type { Metadata } from "next";
import "@/styles/globals.css";

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
    <html lang="en" className="dark">
      <body className="min-h-screen antialiased">{children}</body>
    </html>
  );
}
