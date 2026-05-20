import * as React from "react";
import { JobLibrary } from "@/components/compose/job-library";
import { Composer } from "@/components/compose/composer";
import { parseFromSearchParams } from "@/lib/compose/state";
import { isFunctionId, type FunctionId } from "@/lib/compose/functions";
import { api } from "@/lib/api-client";
import { getCurrentUser } from "@/lib/auth";
import type { SKU } from "@/lib/types";

interface PageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

/**
 * /compose — v3 front door.
 *
 * Default: the 100-job library, browseable by function. Click any card to
 * land on /compose/build?job=<id> for the slot-based composition.
 *
 * Back-compat: any URL with `archetype=` (or any v2 prompt/level/etc params
 * with no `job=` and no library-only intent) falls through to the v2 wizard
 * so existing docs deep-links keep working.
 */
export default async function ComposePage({ searchParams }: PageProps) {
  const params = await searchParams;

  // Back-compat fallthrough — if any v2 wizard params are present, render
  // the wizard. We treat 'archetype' as the unambiguous v2 marker.
  const looksLikeV2Deeplink =
    typeof params.archetype === "string" ||
    typeof params.prompt === "string";

  if (looksLikeV2Deeplink) {
    const state = parseFromSearchParams(params);
    const [user, skus] = await Promise.all([
      getCurrentUser(),
      safeListSkus(),
    ]);
    const plan: "anonymous" | "free" | "paid" = user
      ? user.role === "owner" || user.role === "admin"
        ? "paid"
        : "free"
      : "anonymous";
    return <Composer initialState={state} skus={skus} plan={plan} />;
  }

  const rawFn = params.fn;
  const initialFn: FunctionId | null =
    typeof rawFn === "string" && isFunctionId(rawFn) ? rawFn : null;

  return (
    <div className="relative isolate">
      <div
        aria-hidden
        className="absolute inset-0 -z-10 overflow-hidden rounded-sm"
      >
        <div className="absolute inset-0 compose-grid" />
        <div className="absolute inset-0 compose-grain pointer-events-none" />
        <div
          className="absolute inset-0"
          style={{
            background:
              "radial-gradient(60% 60% at 30% 20%, rgba(91,141,239,0.06), transparent 60%)",
          }}
        />
      </div>
      <div className="relative max-w-6xl mx-auto px-4 sm:px-8 py-8 sm:py-12">
        <span className="compose-bracket tl" aria-hidden />
        <span className="compose-bracket tr" aria-hidden />
        <span className="compose-bracket bl" aria-hidden />
        <span className="compose-bracket br" aria-hidden />

        <header className="mb-10 flex flex-col gap-3">
          <span className="readout-label">OWERA · AGENTIC · JOB LIBRARY</span>
          <h1
            className="text-[2.2rem] sm:text-[3rem] leading-tight"
            style={{ fontFamily: "var(--font-display)" }}
          >
            <em>Hire</em> an agent.
            <br />
            <span className="text-[var(--color-ink-dim)]">
              100 jobs, ten functions, one composer.
            </span>
          </h1>
          <p className="text-sm text-[var(--color-ink-dim)] max-w-2xl">
            Browse what you can hire. Each job is a real outcome an SMB pays
            for, priced by accepted result. Click any card to inspect, edit,
            and hire the agent that runs it.
          </p>
        </header>

        <JobLibrary initialFunction={initialFn} />

        <div className="mt-16 pt-4 border-t border-[var(--color-rule)] flex items-center justify-between">
          <span className="readout-label">OWERA · AGENTIC · NO. 01</span>
          <span className="readout-label">
            POST{" "}
            <a
              href="/api/compose"
              className="underline decoration-[var(--color-stop-fill)] decoration-1 underline-offset-4 hover:text-[var(--color-ink)]"
            >
              /api/compose
            </a>{" "}
            FOR THE SAME SURFACE
          </span>
        </div>
      </div>
    </div>
  );
}

async function safeListSkus(): Promise<SKU[]> {
  try {
    return await api.listSkus();
  } catch {
    return [];
  }
}
