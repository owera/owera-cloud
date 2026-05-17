import * as React from "react";
import Link from "next/link";
import { AuthGuard } from "@/components/auth-guard";

interface NavItem {
  href: string;
  label: string;
  hint: string;
}

const NAV: ReadonlyArray<NavItem> = [
  { href: "/dashboard", label: "OVERVIEW", hint: "Usage & recent jobs" },
  { href: "/jobs", label: "JOBS", hint: "Submit & track" },
  { href: "/usage", label: "USAGE", hint: "Current period meter" },
  { href: "/billing", label: "BILLING", hint: "Stripe portal" },
  { href: "/api-keys", label: "API KEYS", hint: "Manage secrets" },
  { href: "/support", label: "SUPPORT", hint: "Tickets & docs" },
];

export default async function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthGuard>
      {(user) => (
        <div className="min-h-screen grid grid-cols-[14rem_1fr]">
          <aside className="border-r border-[var(--color-border)] bg-[var(--color-card)] sticky top-0 h-screen flex flex-col">
            <div className="px-4 py-4 border-b border-[var(--color-border)]">
              <div className="font-mono text-sm font-semibold tracking-tight">
                OWERA · AGENTIC
              </div>
              <div className="mt-1 text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
                {user.tenantId}
              </div>
            </div>

            <nav className="flex-1 px-2 py-3 space-y-0.5">
              {NAV.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className="block rounded px-2 py-1.5 hover:bg-[var(--color-muted)] transition-colors group"
                >
                  <div className="font-mono text-xs tracking-wide text-[var(--color-foreground)]">
                    {item.label}
                  </div>
                  <div className="text-[10px] text-[var(--color-muted-foreground)] group-hover:text-[var(--color-foreground)]">
                    {item.hint}
                  </div>
                </Link>
              ))}
            </nav>

            <div className="px-4 py-3 border-t border-[var(--color-border)] text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
              <div>SIGNED IN AS</div>
              <div className="font-mono text-[var(--color-foreground)] mt-0.5 normal-case tracking-normal">
                {user.email}
              </div>
            </div>
          </aside>

          <main className="px-6 py-6">{children}</main>
        </div>
      )}
    </AuthGuard>
  );
}
