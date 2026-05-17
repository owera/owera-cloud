import { loadComponents, publicComponents } from "@/lib/components";
import { loadIncidents } from "@/lib/incidents";
import { StatusBoard } from "@/components/StatusBoard";
import { fetchSnapshot } from "../../lib/fetch-snapshot";

// Re-render the SSR shell every 30s so even uncached crawlers (and the
// first paint) get a recent snapshot. The client polls every 30s for
// live updates after hydration.
export const revalidate = 30;

export default async function StatusPage() {
  const componentsFile = await loadComponents();
  const components = publicComponents(componentsFile);
  const groups = componentsFile.groups.filter((g) =>
    g.members.some((id) => components.some((c) => c.id === id)),
  );
  const incidents = await loadIncidents();
  const snapshotUrl =
    process.env.NEXT_PUBLIC_SNAPSHOT_URL ??
    "https://snapshots.owera.ai/health/latest.json";
  const initial = await fetchSnapshot(snapshotUrl);

  return (
    <main className="container">
      <StatusBoard components={components} groups={groups} initial={initial} />

      <section className="incidents">
        <h2>Recent incidents</h2>
        {incidents.length === 0 ? (
          <div className="empty">No incidents in the last 90 days.</div>
        ) : (
          incidents.map((inc) => (
            <article className="incident" key={inc.slug}>
              <h3>{inc.title}</h3>
              <div className="meta">
                {inc.severity.toUpperCase()} ·{" "}
                {new Date(inc.started_at).toUTCString()}
                {inc.resolved_at
                  ? ` · resolved ${new Date(inc.resolved_at).toUTCString()}`
                  : ` · ${inc.status}`}
              </div>
              <pre>{inc.body}</pre>
            </article>
          ))
        )}
      </section>

      <footer className="footer">
        <span>status.owera.ai</span>
        <span>
          <a href="https://owera.ai">owera.ai</a> ·{" "}
          <a href="https://app.owera.ai">app.owera.ai</a>
        </span>
      </footer>
    </main>
  );
}
