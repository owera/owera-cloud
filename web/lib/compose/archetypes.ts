// Job archetypes — the six "outcomes" a user can hire Owera Agentic to deliver.
//
// Each archetype is a structured shape: a slug, a human title and one-line
// description, a default Quality level (the old complexity slider), a default
// SKU, and a list of structured fields that step 2 of the composer renders.
//
// Treat these as the menu. The catch-all "custom" archetype preserves the
// previous prompt-with-slider flow for power users who already know what they
// want.

import type { ComplexityLevel } from "./levels";

export type ArchetypeId =
  | "research"
  | "triage"
  | "brief"
  | "build"
  | "watch"
  | "custom";

export type ArchetypeFieldKind =
  | "long-text"
  | "short-text"
  | "select"
  | "multi-select"
  | "toggle"
  | "url";

export interface ArchetypeField {
  /** Stable id; key in inputs payload. */
  key: string;
  /** Label shown above the field. */
  label: string;
  /** Optional hint shown beneath the label. */
  hint?: string;
  kind: ArchetypeFieldKind;
  /** For select / multi-select. */
  options?: ReadonlyArray<{ value: string; label: string }>;
  placeholder?: string;
  /** Initial value seeded when the archetype is first picked. */
  initial?: string | string[] | boolean;
  /** Whether this field is required to leave step 2. */
  required?: boolean;
  /** When true, this field IS the prompt that goes to the agent. */
  isPrimaryPrompt?: boolean;
}

export interface Archetype {
  id: ArchetypeId;
  /** Plain-language name shown on the card. */
  name: string;
  /** One-sentence tagline. */
  tagline: string;
  /** Two-line description used in the card and review summary. */
  description: string;
  /** SKU slug this archetype hires by default. */
  defaultSku: string;
  /** Default complexity stop. */
  defaultLevel: ComplexityLevel;
  /** Step 2 fields (in order). */
  fields: ReadonlyArray<ArchetypeField>;
  /** Suggested cadence — "once" for ad-hoc, "daily"/"weekly" for repeating. */
  suggestedCadence: "once" | "daily" | "weekly" | "any";
  /** A glyph for the card (single character — kept simple, no icon dep). */
  glyph: string;
}

export const ARCHETYPES: ReadonlyArray<Archetype> = [
  {
    id: "research",
    name: "Research a topic",
    tagline: "A memo with citations.",
    description:
      "Multi-source synthesis on a topic you describe. Returns a structured memo with cited sources.",
    defaultSku: "campaign-swarm",
    defaultLevel: "advanced",
    suggestedCadence: "once",
    glyph: "◇",
    fields: [
      {
        key: "topic",
        label: "Topic or question",
        hint: "Be specific. The narrower the question, the better the memo.",
        kind: "long-text",
        placeholder:
          "e.g. State of pgvector vs Pinecone vs Qdrant — pricing, throughput, operational story.",
        required: true,
        isPrimaryPrompt: true,
      },
      {
        key: "sources",
        label: "Source types",
        kind: "multi-select",
        options: [
          { value: "web", label: "Web search" },
          { value: "papers", label: "Academic papers" },
          { value: "news", label: "News" },
          { value: "docs", label: "Vendor docs" },
        ],
        initial: ["web", "papers"],
      },
      {
        key: "format",
        label: "Output format",
        kind: "select",
        options: [
          { value: "memo", label: "Memo (300–500 words)" },
          { value: "bullets", label: "Bullets" },
          { value: "comparison-table", label: "Comparison table" },
          { value: "deep-dive", label: "Deep dive (1500+ words)" },
        ],
        initial: "memo",
      },
    ],
  },
  {
    id: "triage",
    name: "Triage incoming items",
    tagline: "Sort, label, route.",
    description:
      "Classify and route incoming items — inbox messages, support tickets, leads, alerts — by rules you describe in plain language.",
    defaultSku: "triage-watch",
    defaultLevel: "standard",
    suggestedCadence: "daily",
    glyph: "△",
    fields: [
      {
        key: "source",
        label: "Source",
        hint: "URL, RSS feed, mailbox label, or inbox name.",
        kind: "short-text",
        placeholder: "e.g. inbox:support@owera.ai · OR · rss://example.com/feed",
        required: true,
      },
      {
        key: "criteria",
        label: "How to triage",
        hint: "Describe the rules. The agent will follow them.",
        kind: "long-text",
        placeholder:
          'e.g. "Urgent if production-down or billing-blocked. Otherwise route by product area. Always reply within 1h."',
        required: true,
        isPrimaryPrompt: true,
      },
      {
        key: "action",
        label: "What to do with each item",
        kind: "select",
        options: [
          { value: "label-only", label: "Just label — no reply" },
          { value: "draft-reply", label: "Draft a reply for me" },
          { value: "auto-route", label: "Route to the right person" },
          { value: "auto-respond", label: "Auto-respond (high confidence)" },
        ],
        initial: "draft-reply",
      },
    ],
  },
  {
    id: "brief",
    name: "Daily / weekly brief",
    tagline: "What you need to know.",
    description:
      "A short briefing on a topic, source, or feed. Cheap to run, perfect on a schedule. Lands in your inbox or dashboard.",
    defaultSku: "campaign-swarm",
    defaultLevel: "standard",
    suggestedCadence: "daily",
    glyph: "◯",
    fields: [
      {
        key: "topic",
        label: "What should the brief cover?",
        kind: "long-text",
        placeholder:
          "e.g. New AI papers + repos shipped in the last 24h relevant to small-model finetuning.",
        required: true,
        isPrimaryPrompt: true,
      },
      {
        key: "length",
        label: "Length",
        kind: "select",
        options: [
          { value: "headlines", label: "Headlines only (5 bullets)" },
          { value: "tldr", label: "TL;DR (300 words)" },
          { value: "full", label: "Full brief (1000 words)" },
        ],
        initial: "tldr",
      },
      {
        key: "tone",
        label: "Tone",
        kind: "select",
        options: [
          { value: "neutral", label: "Neutral" },
          { value: "punchy", label: "Punchy / opinionated" },
          { value: "technical", label: "Technical / dry" },
        ],
        initial: "neutral",
      },
    ],
  },
  {
    id: "build",
    name: "Build something",
    tagline: "Code, with tests.",
    description:
      "Generate a working artifact — a function, a script, a page — with an eval pass so the result actually runs.",
    defaultSku: "campaign-swarm",
    defaultLevel: "expert",
    suggestedCadence: "once",
    glyph: "▢",
    fields: [
      {
        key: "goal",
        label: "What should it do?",
        kind: "long-text",
        placeholder:
          "e.g. TypeScript function that deduplicates an array of objects by a key. Include 5 unit tests; run them.",
        required: true,
        isPrimaryPrompt: true,
      },
      {
        key: "language",
        label: "Language / runtime",
        kind: "select",
        options: [
          { value: "typescript", label: "TypeScript" },
          { value: "python", label: "Python" },
          { value: "go", label: "Go" },
          { value: "shell", label: "Shell / bash" },
          { value: "sql", label: "SQL" },
          { value: "other", label: "Other (specify in goal)" },
        ],
        initial: "typescript",
      },
      {
        key: "tests",
        label: "Write & run tests",
        kind: "toggle",
        initial: true,
      },
    ],
  },
  {
    id: "watch",
    name: "Watch & alert",
    tagline: "Tell me when it happens.",
    description:
      "Monitor a feed or page on a schedule. Send an alert when conditions you describe in plain language are met.",
    defaultSku: "triage-watch",
    defaultLevel: "standard",
    suggestedCadence: "daily",
    glyph: "◈",
    fields: [
      {
        key: "source",
        label: "What to watch",
        hint: "URL, RSS feed, or named source.",
        kind: "url",
        placeholder: "e.g. https://news.ycombinator.com/newest",
        required: true,
      },
      {
        key: "trigger",
        label: "Alert me when…",
        hint: "Describe the condition. Vague is fine — the agent will judge.",
        kind: "long-text",
        placeholder:
          'e.g. "A new post mentions our product OR a competitor by name."',
        required: true,
        isPrimaryPrompt: true,
      },
      {
        key: "alert_via",
        label: "Send alert via",
        kind: "select",
        options: [
          { value: "dashboard", label: "Owera dashboard only" },
          { value: "email", label: "Email" },
          { value: "slack", label: "Slack" },
          { value: "webhook", label: "Webhook" },
        ],
        initial: "dashboard",
      },
    ],
  },
  {
    id: "custom",
    name: "Custom",
    tagline: "Open prompt + full controls.",
    description:
      "Free-form prompt with every knob exposed. For when you already know what you want and just need the surface.",
    defaultSku: "campaign-swarm",
    defaultLevel: "advanced",
    suggestedCadence: "any",
    glyph: "✶",
    fields: [
      {
        key: "prompt",
        label: "Prompt",
        kind: "long-text",
        placeholder:
          "Anything. Tools, budget, and the quality dial below let you shape execution.",
        required: true,
        isPrimaryPrompt: true,
      },
    ],
  },
];

export function getArchetype(id: ArchetypeId): Archetype {
  const found = ARCHETYPES.find((a) => a.id === id);
  if (!found) throw new Error(`Unknown archetype: ${id}`);
  return found;
}

export function isArchetypeId(v: unknown): v is ArchetypeId {
  return (
    typeof v === "string" &&
    ARCHETYPES.some((a) => a.id === v)
  );
}

/** Materialize the initial inputs for an archetype (used when first selected). */
export function archetypeInitialInputs(
  id: ArchetypeId,
): Record<string, string | string[] | boolean> {
  const out: Record<string, string | string[] | boolean> = {};
  for (const f of getArchetype(id).fields) {
    if (f.initial !== undefined) out[f.key] = f.initial;
    else if (f.kind === "multi-select") out[f.key] = [];
    else if (f.kind === "toggle") out[f.key] = false;
    else out[f.key] = "";
  }
  return out;
}
