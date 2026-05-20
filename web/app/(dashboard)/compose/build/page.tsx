import { notFound, redirect } from "next/navigation";
import { JobCardComposer } from "@/components/compose/job-card";
import { getJobBlueprint } from "@/lib/compose/catalog";
import { parseFromSearchParams } from "@/lib/compose/state";
import { api } from "@/lib/api-client";
import { getCurrentUser } from "@/lib/auth";
import type { SKU } from "@/lib/types";

interface PageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

/**
 * /compose/build?job=<id> — the slot-based card composer.
 *
 * Hydrates from the catalog blueprint, then renders the editable contract
 * card. Submitting POSTs to /api/compose with the v2 JSON shape + slot
 * metadata; humans and agents go through the same code path.
 */
export default async function ComposeBuildPage({ searchParams }: PageProps) {
  const params = await searchParams;
  const jobIdRaw = params.job;
  const jobId = typeof jobIdRaw === "string" ? jobIdRaw : undefined;

  if (!jobId) {
    // No job id — back to the library.
    redirect("/compose");
  }
  const blueprint = getJobBlueprint(jobId);
  if (!blueprint) notFound();

  const state = parseFromSearchParams(params);
  const [user, skus] = await Promise.all([getCurrentUser(), safeListSkus()]);
  const plan: "anonymous" | "free" | "paid" = user
    ? user.role === "owner" || user.role === "admin"
      ? "paid"
      : "free"
    : "anonymous";

  return (
    <JobCardComposer
      blueprint={blueprint}
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
