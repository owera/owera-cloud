import { NextResponse } from "next/server";
import { DOCS } from "@/lib/docs";

/**
 * /llms.txt — agent-friendly site index.
 *
 * Follows the emerging llms.txt convention: a markdown page that lists every
 * documentation entry with a one-line summary so an agent can plan a fetch
 * traversal in one round-trip.
 */
export function GET() {
  const lines: string[] = [];
  lines.push("# Owera Agentic — for agents");
  lines.push("");
  lines.push(
    "> Owera Agentic is a managed-agent platform with a complexity slider. Drag the slider to control depth, tools, budget, and pricing tier; or POST the same JSON shape via /api/compose.",
  );
  lines.push("");
  lines.push("## Surfaces");
  lines.push("- [POST /api/compose](/api/compose): submit a job from a JSON config");
  lines.push("- [GET /api/compose](/api/compose): inspect a state + estimate");
  lines.push(
    "- [GET /api/compose/schema](/api/compose/schema): JSON Schema for the request body",
  );
  lines.push("- [GET /compose](/compose): human-facing slider UI for the same shape");
  lines.push("");
  lines.push("## Documentation");
  for (const d of DOCS) {
    lines.push(`- [${d.title}](${d.href}): ${d.summary}`);
  }
  lines.push("");
  lines.push("## Conventions");
  lines.push(
    "- URL search params on /compose round-trip with the JSON body of /api/compose.",
  );
  lines.push(
    "- All error responses share `{ code, message, requestId? }`; validation failures add `issues: [{ path, message }]`.",
  );
  lines.push(
    "- Cost estimates are ranges (centsLow..centsHigh), not point values.",
  );

  return new NextResponse(lines.join("\n") + "\n", {
    status: 200,
    headers: {
      "content-type": "text/markdown; charset=utf-8",
      "cache-control": "public, max-age=300",
    },
  });
}
