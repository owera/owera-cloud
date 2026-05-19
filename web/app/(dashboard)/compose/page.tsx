import * as React from "react";
import { Composer } from "@/components/compose/composer";
import { parseFromSearchParams } from "@/lib/compose/state";
import { api } from "@/lib/api-client";
import { getCurrentUser } from "@/lib/auth";
import type { SKU } from "@/lib/types";

interface PageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

/**
 * /compose — the slider front door, v2 (four-step composer).
 *
 * URL search params seed the initial state so deep-links from docs land on
 * the right archetype + Quality dial. The same parser runs here (SSR) and
 * in /api/compose so a dragged-slider job and a POSTed JSON job exercise the
 * same code path.
 */
export default async function ComposePage({ searchParams }: PageProps) {
  const params = await searchParams;
  const state = parseFromSearchParams(params);

  const [user, skus] = await Promise.all([getCurrentUser(), safeListSkus()]);

  // For now: signed-in = "free" plan unless explicitly paid via session metadata.
  // The /api/compose route does the authoritative tier check on submit.
  const plan: "anonymous" | "free" | "paid" = user
    ? user.role === "owner" || user.role === "admin"
      ? "paid"
      : "free"
    : "anonymous";

  return <Composer initialState={state} skus={skus} plan={plan} />;
}

async function safeListSkus(): Promise<SKU[]> {
  try {
    return await api.listSkus();
  } catch {
    return [];
  }
}
