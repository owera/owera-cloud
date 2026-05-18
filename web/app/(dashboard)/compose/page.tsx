import * as React from "react";
import { ComposeSurface } from "@/components/compose/compose-surface";
import { parseFromSearchParams } from "@/lib/compose/state";
import { api } from "@/lib/api-client";
import { getCurrentUser } from "@/lib/auth";
import type { SKU } from "@/lib/types";

interface PageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

/**
 * /compose — the slider front door.
 *
 * URL is the source of truth. The same parser runs here (SSR) and in
 * /api/compose so dragging the slider, copy-pasting a link, or POSTing JSON
 * all produce the same job.
 */
export default async function ComposePage({ searchParams }: PageProps) {
  const params = await searchParams;
  const state = parseFromSearchParams(params);

  const [user, skus] = await Promise.all([
    getCurrentUser(),
    safeListSkus(),
  ]);

  // For now: signed-in = "free" plan unless explicitly paid via session metadata.
  // The /api/compose route does the authoritative tier check on submit.
  const plan: "anonymous" | "free" | "paid" = user
    ? user.role === "owner" || user.role === "admin"
      ? "paid"
      : "free"
    : "anonymous";

  return (
    <ComposeSurface
      initialState={state}
      skus={skus}
      plan={plan}
    />
  );
}

async function safeListSkus(): Promise<SKU[]> {
  try {
    return await api.listSkus();
  } catch {
    return [];
  }
}
