import { NextResponse, type NextRequest } from "next/server";
import {
  composeStateToJobCreate,
  fromJson,
  levelRequiresAuth,
  levelRequiresPaidPlan,
  parseFromSearchParams,
  toJson,
  toSearchParams,
} from "@/lib/compose/state";
import { validateComposeJson } from "@/lib/compose/schema";
import { estimate } from "@/lib/compose/estimate";
import { api, ApiClientError } from "@/lib/api-client";
import { getCurrentUser } from "@/lib/auth";

/**
 * GET /api/compose?level=...&prompt=...
 *
 * Returns the same state-as-JSON the slider would produce. Agents call this to
 * inspect the surface they would POST to. With `?api=1` on the page route, the
 * server returns 303 here so curl-style flows just work.
 */
export async function GET(req: NextRequest) {
  const params = req.nextUrl.searchParams;
  const state = parseFromSearchParams(params);
  const est = estimate(state);
  return NextResponse.json(
    {
      state: toJson(state),
      estimate: est,
      schema: "/api/compose/schema",
      docs: "/docs/reference/api",
    },
    { headers: { "cache-control": "no-store" } },
  );
}

/**
 * POST /api/compose
 *
 * Body: ComposeJson. Validated against the published JSON Schema. On success,
 * runs the same `api.submitJob` the slider does and returns `{ job_id, shareUrl }`.
 */
export async function POST(req: NextRequest) {
  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return errJson(400, "invalid_json", "Body is not valid JSON.");
  }
  const v = validateComposeJson(body);
  if (!v.ok) {
    return NextResponse.json(
      { code: "invalid_request", message: "Validation failed.", issues: v.issues },
      { status: 400 },
    );
  }
  const state = fromJson(v.value);

  // Server-side tier gate. NEVER trust URL/JSON alone.
  const user = await getCurrentUser();
  if (levelRequiresAuth(state.level) && !user) {
    return errJson(
      401,
      "auth_required",
      `Level "${state.level}" requires authentication.`,
    );
  }
  if (
    levelRequiresPaidPlan(state.level) &&
    user &&
    user.role !== "owner" &&
    user.role !== "admin"
  ) {
    return errJson(
      402,
      "upgrade_required",
      `Level "${state.level}" requires a paid plan.`,
    );
  }

  try {
    const payload = composeStateToJobCreate(state);
    const res = await api.submitJob(payload);
    const shareUrl = `/jobs/${encodeURIComponent(res.jobId)}?from=compose&level=${state.level}`;
    return NextResponse.json(
      {
        job_id: res.jobId,
        status: res.status,
        shareUrl,
        rerunUrl: `/compose?${toSearchParams(state).toString()}`,
        estimate: estimate(state),
      },
      { status: 202 },
    );
  } catch (err) {
    if (err instanceof ApiClientError) {
      return NextResponse.json(
        { code: err.code, message: err.message, requestId: err.requestId },
        { status: err.status },
      );
    }
    const message = err instanceof Error ? err.message : "Unknown error";
    return errJson(500, "internal_error", message);
  }
}

function errJson(status: number, code: string, message: string) {
  return NextResponse.json({ code, message }, { status });
}
