import { readFile, readdir } from "node:fs/promises";
import path from "node:path";

export type IncidentMeta = {
  id: string;
  title: string;
  status: "investigating" | "identified" | "monitoring" | "resolved";
  severity: "sev1" | "sev2" | "sev3";
  affected_components: string[];
  started_at: string;
  resolved_at?: string;
};

export type Incident = IncidentMeta & {
  slug: string;
  body: string;
};

const INCIDENTS_DIR = path.resolve(process.cwd(), "..", "incidents");

// Tiny front-matter parser. Pulled in instead of gray-matter to keep the
// dep surface and bundle size minimal for a page that is supposed to
// outlive everything it monitors.
function parseFrontMatter(raw: string): { meta: IncidentMeta; body: string } | null {
  const match = /^---\n([\s\S]*?)\n---\n?([\s\S]*)$/.exec(raw);
  if (!match) return null;
  const head = match[1] ?? "";
  const body = match[2] ?? "";
  const meta: Record<string, unknown> = {};
  for (const line of head.split("\n")) {
    const idx = line.indexOf(":");
    if (idx === -1) continue;
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (value.startsWith("[") && value.endsWith("]")) {
      meta[key] = value
        .slice(1, -1)
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
    } else {
      meta[key] = value.replace(/^["']|["']$/g, "");
    }
  }
  return { meta: meta as unknown as IncidentMeta, body };
}

export async function loadIncidents(): Promise<Incident[]> {
  let names: string[];
  try {
    names = await readdir(INCIDENTS_DIR);
  } catch {
    return [];
  }
  const out: Incident[] = [];
  for (const name of names) {
    if (!name.endsWith(".md")) continue;
    if (name === "TEMPLATE.md") continue;
    const raw = await readFile(path.join(INCIDENTS_DIR, name), "utf8");
    const parsed = parseFrontMatter(raw);
    if (!parsed) continue;
    out.push({
      ...parsed.meta,
      slug: name.replace(/\.md$/, ""),
      body: parsed.body.trim(),
    });
  }
  out.sort((a, b) => (a.started_at < b.started_at ? 1 : -1));
  return out;
}
