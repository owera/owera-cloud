"use client";

import * as React from "react";
import { getArchetype, type ArchetypeField } from "@/lib/compose/archetypes";
import { ComplexitySlider } from "@/components/ui/slider";
import { CostPreview } from "../cost-preview";
import { UpsellGate } from "../upsell-gate";
import {
  defaultsForStop,
  levelRequiresAuth,
  levelRequiresPaidPlan,
  type ComposeState,
} from "@/lib/compose/state";
import { estimate } from "@/lib/compose/estimate";
import type { SKU } from "@/lib/types";
import type { ComplexityLevel } from "@/lib/compose/levels";

interface StepDetailsProps {
  state: ComposeState;
  setState: React.Dispatch<React.SetStateAction<ComposeState>>;
  skus: ReadonlyArray<SKU>;
  plan: "anonymous" | "free" | "paid";
}

export function StepDetails({
  state,
  setState,
  skus,
  plan,
}: StepDetailsProps) {
  const arche = getArchetype(state.archetype);

  const gateAt: ComplexityLevel | null = React.useMemo(() => {
    if (plan === "paid") return null;
    if (plan === "free") return "expert";
    return "advanced";
  }, [plan]);

  const isGated =
    plan === "paid"
      ? false
      : plan === "free"
        ? levelRequiresPaidPlan(state.level)
        : levelRequiresAuth(state.level);

  const est = React.useMemo(
    () => estimate(state, skus),
    [state, skus],
  );

  function patchInput(
    key: string,
    value: string | string[] | boolean,
  ) {
    setState((prev) => ({ ...prev, inputs: { ...prev.inputs, [key]: value } }));
  }

  function setLevel(next: ComplexityLevel) {
    setState((prev) => {
      const recipe = defaultsForStop(next);
      return {
        ...prev,
        level: next,
        // Re-seed recipe-level fields when the dial moves so tools/budget
        // make sense for the new stop. SKU is sticky if the user changed it.
        sku: prev.sku || recipe.sku,
        tools: recipe.tools,
        budget: recipe.budget,
      };
    });
  }

  return (
    <div className="flex flex-col gap-10">
      {/* Archetype-specific fields. */}
      <section className="flex flex-col gap-5">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
          <span className="readout-label">{arche.name}</span>
          <span className="readout-label">02 · DETAILS</span>
        </header>

        {arche.fields.map((field) => (
          <ArchetypeFieldInput
            key={field.key}
            field={field}
            value={state.inputs[field.key]}
            onChange={(v) => patchInput(field.key, v)}
          />
        ))}
      </section>

      {/* Quality dial. */}
      <section>
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2 mb-6">
          <span className="readout-label">Quality dial</span>
          <span className="readout-label text-[var(--color-ink)]">
            {state.level.toUpperCase()}
          </span>
        </header>

        <ComplexitySlider
          value={state.level}
          onChange={setLevel}
          gateAt={gateAt}
        />
      </section>

      {/* Cost & latency preview. */}
      <section>
        <CostPreview estimate={est} />
      </section>

      {isGated && (
        <UpsellGate
          level={state.level}
          unlocks={state.level}
          variant={plan === "anonymous" ? "signin" : "checkout"}
        />
      )}
    </div>
  );
}

/* ---------------- Field renderer ---------------- */

function ArchetypeFieldInput({
  field,
  value,
  onChange,
}: {
  field: ArchetypeField;
  value: string | string[] | boolean | undefined;
  onChange: (v: string | string[] | boolean) => void;
}) {
  const baseLabel = (
    <label className="flex flex-col gap-1.5">
      <span className="flex items-center justify-between">
        <span className="readout-label">
          {field.label}
          {field.required && (
            <span className="text-[var(--color-stop-fill)] ml-1">•</span>
          )}
        </span>
        {field.isPrimaryPrompt && (
          <span className="readout-label text-[var(--color-stop-fill)]/80">
            PRIMARY
          </span>
        )}
      </span>
      {field.hint && (
        <span
          className="italic text-xs text-[var(--color-ink-dim)]"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {field.hint}
        </span>
      )}
    </label>
  );

  const inputClass = [
    "w-full rounded-sm border bg-[rgba(0,0,0,0.25)]",
    "border-[var(--color-rule)] focus:border-[var(--color-stop-fill)]",
    "px-4 py-2.5 text-base font-sans text-[var(--color-ink)]",
    "placeholder:text-[var(--color-ink-dim)]/70 placeholder:italic",
    "focus:outline-none focus:ring-1 focus:ring-[var(--color-stop-fill)]/30",
    "transition-colors",
  ].join(" ");

  switch (field.kind) {
    case "long-text":
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <textarea
            rows={4}
            maxLength={8000}
            placeholder={field.placeholder}
            required={field.required}
            value={String(value ?? "")}
            onChange={(e) => onChange(e.target.value)}
            className={`${inputClass} resize-y`}
          />
        </div>
      );
    case "short-text":
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <input
            type="text"
            placeholder={field.placeholder}
            required={field.required}
            value={String(value ?? "")}
            onChange={(e) => onChange(e.target.value)}
            className={`${inputClass} h-10`}
          />
        </div>
      );
    case "url":
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <input
            type="url"
            placeholder={field.placeholder}
            required={field.required}
            value={String(value ?? "")}
            onChange={(e) => onChange(e.target.value)}
            className={`${inputClass} h-10 font-mono text-sm`}
          />
        </div>
      );
    case "select":
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <select
            value={String(value ?? field.options?.[0]?.value ?? "")}
            onChange={(e) => onChange(e.target.value)}
            className={`${inputClass} h-10 font-mono text-sm`}
          >
            {field.options?.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      );
    case "multi-select": {
      const arr = Array.isArray(value) ? value : [];
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <div className="flex flex-wrap gap-1.5">
            {field.options?.map((o) => {
              const on = arr.includes(o.value);
              return (
                <button
                  key={o.value}
                  type="button"
                  onClick={() =>
                    onChange(
                      on
                        ? arr.filter((x) => x !== o.value)
                        : [...arr, o.value],
                    )
                  }
                  className={[
                    "px-3 h-8 rounded-sm border text-xs font-mono uppercase tracking-wider",
                    "transition-colors",
                    on
                      ? "bg-[var(--color-stop-fill)]/15 border-[var(--color-stop-fill)] text-[var(--color-stop-fill)]"
                      : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:border-[var(--color-stop-fill)]/40",
                  ].join(" ")}
                >
                  {o.label}
                </button>
              );
            })}
          </div>
        </div>
      );
    }
    case "toggle":
      return (
        <div className="flex flex-col gap-2">
          {baseLabel}
          <label className="inline-flex items-center gap-3 cursor-pointer select-none">
            <span
              className={[
                "relative inline-block w-10 h-5 rounded-full transition-colors",
                value ? "bg-[var(--color-stop-fill)]" : "bg-[var(--color-rule)]",
              ].join(" ")}
            >
              <span
                className={[
                  "absolute top-0.5 size-4 rounded-full bg-[var(--color-ink)] transition-transform",
                  value ? "translate-x-5" : "translate-x-0.5",
                ].join(" ")}
              />
            </span>
            <input
              type="checkbox"
              className="sr-only"
              checked={!!value}
              onChange={(e) => onChange(e.target.checked)}
            />
            <span className="text-sm text-[var(--color-ink-dim)]">
              {value ? "Enabled" : "Disabled"}
            </span>
          </label>
        </div>
      );
  }
}
