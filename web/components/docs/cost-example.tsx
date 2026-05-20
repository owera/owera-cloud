import { defaultsForStop } from "@/lib/compose/state";
import { estimate, formatLatency } from "@/lib/compose/estimate";
import {
  complexityLevelLabel,
  type ComplexityLevel,
} from "@/lib/compose/levels";
import { formatCents } from "@/lib/format";

interface CostExampleProps {
  stop: ComplexityLevel;
  prompt?: string;
}

/**
 * Inline cost-and-latency snapshot for a given stop, computed via the same
 * `estimate()` the live preview uses. Lets docs pages show real numbers
 * without faking them.
 */
export function CostExample({ stop, prompt = "" }: CostExampleProps) {
  const state = { ...defaultsForStop(stop), prompt };
  const est = estimate(state);
  return (
    <span className="font-mono text-xs text-[var(--color-foreground)] inline-flex items-center gap-1.5 align-baseline">
      <span className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
        {complexityLevelLabel(stop)}
      </span>
      <span>
        {formatCents(est.centsLow)}–{formatCents(est.centsHigh)}
      </span>
      <span className="text-[var(--color-muted-foreground)]">·</span>
      <span>{formatLatency(est.p50ms)}</span>
    </span>
  );
}
