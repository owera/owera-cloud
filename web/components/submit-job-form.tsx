"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { api, ApiClientError } from "@/lib/api-client";

interface SubmitJobFormProps {
  skuOptions?: ReadonlyArray<string>;
}

const DEFAULT_SKUS = ["triage-watch", "campaign-swarm"] as const;

export function SubmitJobForm({ skuOptions = DEFAULT_SKUS }: SubmitJobFormProps) {
  const router = useRouter();
  const [sku, setSku] = React.useState<string>(skuOptions[0] ?? "triage-watch");
  const [prompt, setPrompt] = React.useState<string>("");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [submitted, setSubmitted] = React.useState<string | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    setSubmitted(null);
    try {
      const idemKey = `web-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const res = await api.submitJob({
        sku,
        inputs: { prompt },
        idempotencyKey: idemKey,
      });
      setSubmitted(res.jobId);
      setPrompt("");
      router.refresh();
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(`${err.code}: ${err.message}`);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError("Unknown error");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={onSubmit}
      className="flex flex-col gap-2 sm:flex-row sm:items-end"
    >
      <label className="flex flex-col gap-1 text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
        SKU
        <select
          value={sku}
          onChange={(e) => setSku(e.target.value)}
          className="h-9 rounded-md border bg-[var(--color-input)] border-[var(--color-border)] px-2 text-sm font-mono text-[var(--color-foreground)]"
        >
          {skuOptions.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </label>
      <label className="flex-1 flex flex-col gap-1 text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
        PROMPT
        <Input
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder='e.g. "summarise yesterday’s incidents"'
          required
        />
      </label>
      <Button type="submit" variant="primary" disabled={busy || !prompt.trim()}>
        {busy ? "Submitting…" : "Submit job"}
      </Button>
      {submitted && (
        <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-state-succeeded)]">
          Queued · {submitted}
        </span>
      )}
      {error && (
        <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-state-failed)]">
          {error}
        </span>
      )}
    </form>
  );
}
