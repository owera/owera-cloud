import Link from "next/link";
import {
  ARCHETYPES,
  type ArchetypeId,
  type Archetype,
} from "@/lib/compose/archetypes";

export interface TemplateSeed {
  id: string;
  archetype: ArchetypeId;
  name: string;
  tagline: string;
  /** Deep-link query string. Picks up the slider/SKU; structured inputs are
   *  passed via `prompt=...` as a synthesized seed so even agents can replay. */
  href: string;
  /** Optional cadence label shown on the card. */
  cadence?: string;
}

/**
 * A small re-hireable card. Click takes you straight into the composer
 * pre-filled, where you can run-as-is or tweak before hiring.
 */
export function TemplateCard({
  seed,
  hint,
}: {
  seed: TemplateSeed;
  hint?: string;
}) {
  const arche: Archetype | undefined = ARCHETYPES.find(
    (a) => a.id === seed.archetype,
  );
  return (
    <Link
      href={seed.href}
      className="group relative border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.2)] px-4 py-4 flex flex-col gap-2 hover:border-[var(--color-stop-fill)]/50 transition-colors"
    >
      <div className="flex items-start justify-between">
        <span
          className="text-2xl leading-none text-[var(--color-stop-fill)]"
          style={{ fontFamily: "var(--font-display)" }}
          aria-hidden
        >
          {arche?.glyph ?? "✶"}
        </span>
        <span className="readout-label">
          {seed.archetype.toUpperCase()}
        </span>
      </div>
      <span className="font-mono text-sm uppercase tracking-wider text-[var(--color-ink)] mt-1">
        {seed.name}
      </span>
      <span
        className="italic text-sm text-[var(--color-ink-dim)] leading-snug"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {seed.tagline}
      </span>
      <div className="mt-1 flex items-center justify-between text-xs">
        <span className="readout-label">
          {seed.cadence ?? "AD-HOC"}
        </span>
        <span className="text-[var(--color-stop-fill)] group-hover:underline">
          {hint ?? "Re-hire →"}
        </span>
      </div>
    </Link>
  );
}
