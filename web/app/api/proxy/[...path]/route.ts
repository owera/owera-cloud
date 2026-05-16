// /api/proxy/* — forwards client-side calls to api.owera.ai with the user's
// session token attached server-side. Lets browser code call the API without
// exposing the bearer token. Returns whatever the upstream returns, verbatim.

import { NextRequest, NextResponse } from "next/server";
import { getApiToken } from "@/lib/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

// Headers we strip before forwarding upstream (or before returning to client).
const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
]);

async function handle(
  req: NextRequest,
  ctx: { params: Promise<{ path: string[] }> },
) {
  const { path } = await ctx.params;
  const suffix = path.join("/");
  const search = req.nextUrl.search;
  const upstream = `${BASE}/${suffix}${search}`;

  const headers = new Headers();
  req.headers.forEach((v, k) => {
    if (!HOP_BY_HOP.has(k.toLowerCase())) headers.set(k, v);
  });

  const token = await getApiToken();
  if (token) headers.set("authorization", `Bearer ${token}`);

  const body =
    req.method === "GET" || req.method === "HEAD" ? undefined : req.body;

  const res = await fetch(upstream, {
    method: req.method,
    headers,
    body: body as BodyInit | undefined,
    // @ts-expect-error — duplex is required for streaming bodies on Node fetch
    duplex: "half",
    redirect: "manual",
    cache: "no-store",
  });

  const outHeaders = new Headers();
  res.headers.forEach((v, k) => {
    if (!HOP_BY_HOP.has(k.toLowerCase())) outHeaders.set(k, v);
  });

  return new NextResponse(res.body, {
    status: res.status,
    statusText: res.statusText,
    headers: outHeaders,
  });
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const OPTIONS = handle;
