// The ten business functions Owera composes jobs around.
//
// Research note: SMBs don't think in "shapes" (research / triage / brief…);
// they think in "departments." Organize the catalog so users land on
// jobs they recognize from the function rail in <5 seconds.

export type FunctionId =
  | "sales"
  | "marketing"
  | "cs"
  | "ops"
  | "finance"
  | "legal"
  | "people"
  | "it"
  | "product"
  | "founder";

export interface Func {
  id: FunctionId;
  label: string;
  glyph: string;
  /** One-line description used as the section subtitle. */
  blurb: string;
}

export const FUNCTIONS: ReadonlyArray<Func> = [
  {
    id: "sales",
    label: "Sales",
    glyph: "↗",
    blurb: "Outbound, inbound, pipeline, renewals.",
  },
  {
    id: "marketing",
    label: "Marketing",
    glyph: "◐",
    blurb: "Content, signals, reviews, monitoring.",
  },
  {
    id: "cs",
    label: "Customer Success",
    glyph: "◯",
    blurb: "Triage, onboarding, health, save plays.",
  },
  {
    id: "ops",
    label: "Operations",
    glyph: "▢",
    blurb: "Dashboards, vendors, SOPs, audits.",
  },
  {
    id: "finance",
    label: "Finance",
    glyph: "◆",
    blurb: "Collections, close, forecasting, spend.",
  },
  {
    id: "legal",
    label: "Legal",
    glyph: "§",
    blurb: "Redlines, contract register, privacy.",
  },
  {
    id: "people",
    label: "People",
    glyph: "△",
    blurb: "Hiring, onboarding, comp, reviews.",
  },
  {
    id: "it",
    label: "IT & Security",
    glyph: "⌘",
    blurb: "Access, patching, compliance, incidents.",
  },
  {
    id: "product",
    label: "Product",
    glyph: "◈",
    blurb: "Feedback, competitor watch, releases.",
  },
  {
    id: "founder",
    label: "Founder / Exec",
    glyph: "✶",
    blurb: "Brief, inbox, investors, comms.",
  },
];

export function getFunction(id: FunctionId): Func {
  const f = FUNCTIONS.find((x) => x.id === id);
  if (!f) throw new Error(`Unknown function: ${id}`);
  return f;
}

export function isFunctionId(v: unknown): v is FunctionId {
  return typeof v === "string" && FUNCTIONS.some((f) => f.id === v);
}
