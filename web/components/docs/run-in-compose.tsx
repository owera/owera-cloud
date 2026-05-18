import Link from "next/link";
import { Button } from "@/components/ui/button";
import { toSearchParams } from "@/lib/compose/state";
import type { ComplexityLevel } from "@/lib/compose/levels";

interface RunInComposeProps {
  level: ComplexityLevel;
  prompt: string;
  sku?: string;
  label?: string;
}

/**
 * "Try this in compose" deep-link button. Reading leads to doing without a
 * context switch — the docs-to-conversion loop.
 */
export function RunInCompose({
  level,
  prompt,
  sku,
  label = "Try this in compose",
}: RunInComposeProps) {
  const qs = toSearchParams({
    level,
    sku: sku ?? "",
    prompt,
    tools: [],
    budget: {},
  });
  // toSearchParams drops empty sku via the seeded-default check; ensure we
  // explicitly set the prompt + level (which it does).
  const href = `/compose?${qs.toString()}`;
  return (
    <Button asChild variant="primary" size="sm" className="not-prose">
      <Link href={href}>{label}</Link>
    </Button>
  );
}
