import { NextResponse } from "next/server";
import { COMPOSE_JSON_SCHEMA } from "@/lib/compose/schema";

/**
 * GET /api/compose/schema
 *
 * Returns the JSON Schema (Draft 2020-12) for POST /api/compose. Agents can
 * fetch this at discovery time and validate request bodies offline.
 *
 * Served as application/schema+json with long-tail caching — schema only
 * changes on deploy.
 */
export function GET() {
  return new NextResponse(JSON.stringify(COMPOSE_JSON_SCHEMA, null, 2), {
    status: 200,
    headers: {
      "content-type": "application/schema+json; charset=utf-8",
      "cache-control": "public, max-age=300, s-maxage=300",
    },
  });
}
