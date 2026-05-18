import * as React from "react";

type Kind = "tip" | "trust" | "cost" | "warning";

const STYLE: Record<Kind, { border: string; bg: string; label: string }> = {
  tip: {
    border: "border-[var(--color-primary)]/40",
    bg: "bg-[var(--color-primary)]/5",
    label: "TIP",
  },
  trust: {
    border: "border-[var(--color-state-succeeded)]/40",
    bg: "bg-[var(--color-state-succeeded)]/5",
    label: "TRUST",
  },
  cost: {
    border: "border-[var(--color-state-running)]/40",
    bg: "bg-[var(--color-state-running)]/5",
    label: "COST",
  },
  warning: {
    border: "border-[var(--color-state-failed)]/40",
    bg: "bg-[var(--color-state-failed)]/5",
    label: "HEADS-UP",
  },
};

export function Callout({
  kind = "tip",
  children,
}: {
  kind?: Kind;
  children: React.ReactNode;
}) {
  const s = STYLE[kind];
  return (
    <div
      className={`border rounded-md ${s.border} ${s.bg} px-4 py-3 my-4 flex gap-3`}
    >
      <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)] shrink-0 mt-0.5">
        {s.label}
      </span>
      <div className="text-sm text-[var(--color-foreground)] [&_p]:my-0">
        {children}
      </div>
    </div>
  );
}
