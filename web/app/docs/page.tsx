import Link from "next/link";
import { docsBySection, SECTIONS } from "@/lib/docs";

export const metadata = {
  title: "Owera Docs — Guides & API reference",
  description:
    "Owera Agentic documentation. Learn the complexity slider, run your first job in 60 seconds, integrate via the API.",
};

export default function DocsIndex() {
  const grouped = docsBySection();
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        OWERA · DOCS
      </div>
      <h1>Documentation</h1>
      <p className="lede">
        Owera Agentic gives you a slider for complexity and an API for everything
        else. Start with the quickstart, or dive into the concept guides to learn
        what each slider stop means.
      </p>

      <div className="not-prose mt-6 grid grid-cols-1 sm:grid-cols-2 gap-3">
        <Link
          href="/docs/quickstart"
          className="border border-[var(--color-border)] rounded-md bg-[var(--color-card)] px-4 py-3 hover:border-[var(--color-primary)] transition-colors"
        >
          <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-primary)]">
            START HERE
          </div>
          <div className="text-base text-[var(--color-foreground)] mt-1">
            60 seconds to your first job
          </div>
          <div className="text-xs text-[var(--color-muted-foreground)] mt-0.5">
            One prompt. One click. A working agent.
          </div>
        </Link>
        <Link
          href="/docs/concepts/complexity-slider"
          className="border border-[var(--color-border)] rounded-md bg-[var(--color-card)] px-4 py-3 hover:border-[var(--color-primary)] transition-colors"
        >
          <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-primary)]">
            CORE CONCEPT
          </div>
          <div className="text-base text-[var(--color-foreground)] mt-1">
            How the Complexity Slider works
          </div>
          <div className="text-xs text-[var(--color-muted-foreground)] mt-0.5">
            One control, five stops, every job a URL.
          </div>
        </Link>
      </div>

      {SECTIONS.map((section) => {
        const entries = grouped[section];
        if (!entries.length) return null;
        return (
          <section key={section}>
            <h2>{section}</h2>
            <ul>
              {entries.map((d) => (
                <li key={d.slug}>
                  <Link href={d.href}>{d.title}</Link>
                  <span className="text-[var(--color-muted-foreground)]"> — {d.summary}</span>
                </li>
              ))}
            </ul>
          </section>
        );
      })}

      <h2>For agents</h2>
      <p>
        Everything humans see in <code>/compose</code> is exposed as a JSON shape an
        agent can post. Discover the surface in one fetch:
      </p>
      <ul>
        <li>
          <Link href="/llms.txt">/llms.txt</Link> — one-line index of every doc page
        </li>
        <li>
          <Link href="/api/compose/schema">/api/compose/schema</Link> — JSON Schema
          for <code>POST /api/compose</code>
        </li>
        <li>
          <Link href="/docs/reference/api">/docs/reference/api</Link> — request &
          response examples
        </li>
      </ul>
    </>
  );
}
