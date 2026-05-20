"use client";

import * as React from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  complexityLevelLabel,
  type ComplexityLevel,
} from "@/lib/compose/levels";

interface UpsellGateProps {
  /** What stop the user is currently parked at. */
  level: ComplexityLevel;
  /** What stop they would unlock by upgrading (the next gated stop). */
  unlocks: ComplexityLevel;
  /** Anonymous → sign-in. Signed-in free → checkout. */
  variant: "signin" | "checkout";
  signInHref?: string;
}

export function UpsellGate({
  level,
  unlocks,
  variant,
  signInHref = "/sign-in",
}: UpsellGateProps) {
  const isCheckout = variant === "checkout";
  return (
    <div className="border border-[var(--color-primary)]/40 rounded-md bg-[var(--color-primary)]/5 px-4 py-3 flex items-start gap-3">
      <div className="size-2 rounded-full bg-[var(--color-primary)] mt-1.5 shrink-0" />
      <div className="flex-1 min-w-0">
        <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-primary)]">
          Unlock {complexityLevelLabel(unlocks)}
        </div>
        <div className="text-sm text-[var(--color-foreground)] mt-0.5">
          You&apos;re running at <b className="font-mono">{complexityLevelLabel(level)}</b>.{" "}
          {isCheckout
            ? "Upgrade to run multi-agent orchestration, larger budgets, and richer tool sets."
            : "Sign in to try Advanced and beyond — no card required for the 14-day trial."}
        </div>
        <div className="mt-2 flex gap-2">
          {isCheckout ? (
            <Button asChild variant="primary" size="sm">
              <Link href="/billing">Start 14-day trial</Link>
            </Button>
          ) : (
            <Button asChild variant="primary" size="sm">
              <Link href={signInHref}>Sign in to try</Link>
            </Button>
          )}
          <Button asChild variant="ghost" size="sm">
            <Link href="/docs/concepts/complexity-slider">How tiers work</Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
