import { NextResponse } from "next/server";
import { fetchSnapshot } from "../../../../lib/fetch-snapshot";

export const dynamic = "force-dynamic";
export const revalidate = 0;

// The client polls this endpoint every 30s. Doing the fetch server-side
// keeps the snapshot URL configurable per environment and lets us add
// auth / signature verification later without shipping logic to the
// client.
export async function GET() {
  const url =
    process.env.NEXT_PUBLIC_SNAPSHOT_URL ??
    "https://snapshots.owera.ai/health/latest.json";
  const result = await fetchSnapshot(url);
  return NextResponse.json(result, {
    headers: {
      "cache-control": "no-store, max-age=0",
    },
  });
}
