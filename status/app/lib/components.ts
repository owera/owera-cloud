import { readFile } from "node:fs/promises";
import path from "node:path";
import yaml from "js-yaml";

export type ProbeConfig = {
  type: "http" | "aggregate";
  url?: string;
  source?: string;
  expected_status?: number;
  timeout_ms?: number;
  interval_s: number;
  healthy_threshold_pct?: number;
};

export type SLAConfig = {
  uptime_pct: number;
  latency_p95_ms?: number;
  window: string;
};

export type Component = {
  id: string;
  name: string;
  description: string;
  probe: ProbeConfig;
  sla: SLAConfig;
  public: boolean;
  notes?: string;
};

export type ComponentGroup = {
  id: string;
  name: string;
  members: string[];
};

export type ComponentsFile = {
  components: Component[];
  groups: ComponentGroup[];
};

// Resolved at build time; the file lives one level up from status/app/.
const COMPONENTS_PATH = path.resolve(process.cwd(), "..", "components.yaml");

export async function loadComponents(): Promise<ComponentsFile> {
  const raw = await readFile(COMPONENTS_PATH, "utf8");
  const parsed = yaml.load(raw) as ComponentsFile;
  if (!parsed?.components?.length) {
    throw new Error(`components.yaml at ${COMPONENTS_PATH} is empty or malformed`);
  }
  return parsed;
}

export function publicComponents(file: ComponentsFile): Component[] {
  return file.components.filter((c) => c.public);
}
