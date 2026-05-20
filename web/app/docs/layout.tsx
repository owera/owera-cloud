import * as React from "react";
import Link from "next/link";
import { docsBySection, SECTIONS } from "@/lib/docs";

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const grouped = docsBySection();
  return (
    <div className="min-h-screen grid grid-cols-[16rem_1fr] gap-6 max-w-6xl mx-auto px-6 py-6">
      <aside className="sticky top-6 h-fit">
        <Link href="/docs" className="block mb-4">
          <div className="font-mono text-sm font-semibold tracking-tight">
            OWERA · DOCS
          </div>
          <div className="mt-0.5 text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
            Guides & API reference
          </div>
        </Link>

        <nav className="flex flex-col gap-4">
          {SECTIONS.map((section) => {
            const entries = grouped[section];
            if (!entries.length) return null;
            return (
              <div key={section}>
                <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)] mb-1.5 px-2">
                  {section}
                </div>
                <ul className="flex flex-col gap-0.5">
                  {entries.map((d) => (
                    <li key={d.slug}>
                      <Link
                        href={d.href}
                        className="block rounded px-2 py-1 text-sm text-[var(--color-foreground)] hover:bg-[var(--color-muted)] transition-colors"
                      >
                        {d.title}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            );
          })}

          <div className="border-t border-[var(--color-border)] pt-3 mt-2 flex flex-col gap-1 px-2">
            <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
              For agents
            </span>
            <Link
              href="/llms.txt"
              className="text-xs text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
            >
              /llms.txt
            </Link>
            <Link
              href="/api/compose/schema"
              className="text-xs text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
            >
              /api/compose/schema
            </Link>
          </div>
        </nav>
      </aside>

      <main className="min-w-0 max-w-3xl py-2">
        <article className="prose-owera">{children}</article>
      </main>
    </div>
  );
}
