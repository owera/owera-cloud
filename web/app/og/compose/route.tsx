import { ImageResponse } from "next/og";
import { parseFromSearchParams } from "@/lib/compose/state";
import { estimate, formatLatency } from "@/lib/compose/estimate";
import { complexityLevelLabel } from "@/lib/compose/levels";
import { formatCents } from "@/lib/format";
import type { NextRequest } from "next/server";

export const runtime = "edge";

/**
 * GET /og/compose?level=...&prompt=...
 *
 * Renders a 1200x630 social share image with the slider stop badge and cost
 * range. Used as the open-graph image for shareable result URLs.
 */
export function GET(req: NextRequest) {
  const params = req.nextUrl.searchParams;
  const state = parseFromSearchParams(params);
  const est = estimate(state);

  const promptPreview =
    state.prompt.length > 140
      ? state.prompt.slice(0, 137) + "…"
      : state.prompt || "Run an agent at the right level of complexity.";

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          background: "#0a0a0b",
          color: "#e6e6e8",
          padding: "64px 80px",
          fontFamily: "ui-sans-serif, system-ui, sans-serif",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 16,
            fontFamily: "ui-monospace, JetBrains Mono, SF Mono, monospace",
            fontSize: 18,
            color: "#8a8a93",
            letterSpacing: 2,
            textTransform: "uppercase",
          }}
        >
          <div
            style={{
              width: 10,
              height: 10,
              background: "#5b8def",
              borderRadius: 9999,
            }}
          />
          OWERA · AGENTIC
        </div>

        <div style={{ marginTop: 48, display: "flex", flexDirection: "column" }}>
          <div
            style={{
              display: "flex",
              alignSelf: "flex-start",
              padding: "6px 14px",
              border: "1px solid #5b8def",
              borderRadius: 6,
              fontFamily: "ui-monospace, JetBrains Mono, SF Mono, monospace",
              fontSize: 22,
              color: "#5b8def",
              letterSpacing: 4,
            }}
          >
            {complexityLevelLabel(state.level)}
          </div>

          <div
            style={{
              marginTop: 28,
              fontSize: 48,
              lineHeight: 1.15,
              color: "#e6e6e8",
              maxWidth: 1040,
            }}
          >
            {promptPreview}
          </div>
        </div>

        <div style={{ marginTop: "auto", display: "flex", gap: 48 }}>
          <Stat label="ESTIMATED" value={`${formatCents(est.centsLow)} – ${formatCents(est.centsHigh)}`} />
          <Stat label="TYPICAL" value={formatLatency(est.p50ms)} />
          <Stat label="P95" value={formatLatency(est.p95ms)} />
        </div>

        <div
          style={{
            marginTop: 32,
            fontFamily: "ui-monospace, JetBrains Mono, SF Mono, monospace",
            fontSize: 16,
            color: "#6a6a73",
            letterSpacing: 1.5,
          }}
        >
          app.owera.ai/compose
        </div>
      </div>
    ),
    { width: 1200, height: 630 },
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <span
        style={{
          fontFamily: "ui-monospace, JetBrains Mono, SF Mono, monospace",
          fontSize: 12,
          color: "#8a8a93",
          letterSpacing: 2,
        }}
      >
        {label}
      </span>
      <span style={{ fontSize: 28, color: "#e6e6e8" }}>{value}</span>
    </div>
  );
}
