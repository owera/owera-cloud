// Central index of doc pages.
//
// Each entry is the source of truth for the sidebar, the llms.txt agent index,
// search (when wired), and the breadcrumb. Adding a new doc means: create the
// page.tsx under app/docs/<slug>, register it here.

export interface DocEntry {
  slug: string;
  href: string;
  title: string;
  summary: string;
  section: "Get started" | "Concepts" | "Recipes" | "Reference";
  /** Higher = appears later within its section. */
  order: number;
}

export const DOCS: ReadonlyArray<DocEntry> = [
  {
    slug: "quickstart",
    href: "/docs/quickstart",
    title: "Quickstart — 60 seconds to first job",
    summary:
      "Submit your first Owera Agentic job from the slider in under a minute.",
    section: "Get started",
    order: 1,
  },
  {
    slug: "concepts/complexity-slider",
    href: "/docs/concepts/complexity-slider",
    title: "The Complexity Slider",
    summary:
      "One control, five stops, infinite range. How the slider abstracts agent depth, tools, budget, and pricing.",
    section: "Concepts",
    order: 1,
  },
  {
    slug: "concepts/pick-your-stop",
    href: "/docs/concepts/pick-your-stop",
    title: "Pick your stop",
    summary:
      "When to use Simple vs Standard vs Advanced vs Expert vs Custom — with real cost and latency examples.",
    section: "Concepts",
    order: 2,
  },
  {
    slug: "concepts/cost-and-pricing",
    href: "/docs/concepts/cost-and-pricing",
    title: "Cost & pricing model",
    summary:
      "How we estimate cost up-front, what drives the range, and why the slider stop maps to your bill.",
    section: "Concepts",
    order: 3,
  },
  {
    slug: "reference/api",
    href: "/docs/reference/api",
    title: "API reference — POST /api/compose",
    summary:
      "The agent-programmable surface that mirrors the slider. JSON Schema, curl examples, error envelopes.",
    section: "Reference",
    order: 1,
  },
  {
    slug: "reference/skus",
    href: "/docs/reference/skus",
    title: "SKU catalog",
    summary:
      "Every published SKU with inputs, outputs, base price, and an example payload.",
    section: "Reference",
    order: 2,
  },
];

export const SECTIONS: ReadonlyArray<DocEntry["section"]> = [
  "Get started",
  "Concepts",
  "Recipes",
  "Reference",
];

export function docsBySection(): Record<DocEntry["section"], DocEntry[]> {
  const out: Record<DocEntry["section"], DocEntry[]> = {
    "Get started": [],
    Concepts: [],
    Recipes: [],
    Reference: [],
  };
  for (const d of DOCS) out[d.section].push(d);
  for (const s of SECTIONS) out[s].sort((a, b) => a.order - b.order);
  return out;
}
