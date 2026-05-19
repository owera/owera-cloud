import Link from "next/link";
import { Button } from "@/components/ui/button";
import type { ComplexityLevel } from "@/lib/compose/levels";
import type { ArchetypeId } from "@/lib/compose/archetypes";

interface RunInComposeProps {
  level: ComplexityLevel;
  prompt: string;
  sku?: string;
  archetype?: ArchetypeId;
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
  archetype,
  label = "Try this in compose",
}: RunInComposeProps) {
  const qs = new URLSearchParams();
  if (archetype) qs.set("archetype", archetype);
  qs.set("level", level);
  if (sku) qs.set("sku", sku);
  if (prompt) qs.set("prompt", prompt);
  const href = `/compose?${qs.toString()}`;
  return (
    <Button asChild variant="primary" size="sm" className="not-prose">
      <Link href={href}>{label}</Link>
    </Button>
  );
}
