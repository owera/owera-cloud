import { redirect } from "next/navigation";
import Link from "next/link";
import { getCurrentUser } from "@/lib/auth";

export const dynamic = "force-dynamic";

// Root entry. Signed-in users go to /dashboard; everyone else sees an
// inline marketing splash. The splash lives here (not in (marketing)/page)
// because Next 15 prerendering trips on cross-route-group imports.
export default async function RootPage() {
  const user = await getCurrentUser();
  if (user) redirect("/dashboard");

  return (
    <main className="min-h-screen flex flex-col items-center justify-center px-6">
      <div className="max-w-xl w-full">
        <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-muted-foreground)] mb-3">
          owera.ai
        </div>
        <h1 className="font-mono text-3xl font-semibold tracking-tight mb-3">
          Owera Agentic
        </h1>
        <p className="text-sm text-[var(--color-muted-foreground)] leading-relaxed mb-8">
          Agentic work as a managed service. Submit a job, we route it across a
          managed fleet of agents and return signed, auditable outputs.
        </p>

        <div className="flex items-center gap-3">
          <Link
            href="/dashboard"
            className="inline-flex h-9 px-3 items-center rounded-md text-sm font-medium bg-[var(--color-primary)] text-[var(--color-primary-foreground)] hover:opacity-90 transition-opacity"
          >
            Open dashboard
          </Link>
          <Link
            href="/support"
            className="inline-flex h-9 px-3 items-center rounded-md text-sm font-medium border border-[var(--color-border)] hover:bg-[var(--color-muted)] transition-colors"
          >
            Docs &amp; SKUs
          </Link>
        </div>

        <div className="mt-12 grid grid-cols-3 gap-3 text-xs font-mono">
          <Cell label="SLA" value="99.9%" />
          <Cell label="Region" value="us-east" />
          <Cell label="Build" value="dev" />
        </div>
      </div>
    </main>
  );
}

function Cell({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--color-border)] bg-[var(--color-card)] px-3 py-2">
      <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
        {label}
      </div>
      <div className="text-sm text-[var(--color-foreground)]">{value}</div>
    </div>
  );
}
