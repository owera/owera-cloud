// Clerk session middleware.
//
// Runs on every dashboard route to attach the Clerk session (HTTP-only
// cookie) to the request. Public routes — the marketing splash, sign-in,
// and sign-up — bypass the auth check. The /api/proxy/* route is gated
// because it forwards to the upstream Owera API with a server-side token.
//
// Falls back to a pass-through when @clerk/nextjs isn't installed (mock
// dev mode), so contributors without Clerk keys can still boot the app.

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const PUBLIC_ROUTES = [
  "/",
  "/sign-in",
  "/sign-in/(.*)",
  "/sign-up",
  "/sign-up/(.*)",
  "/favicon.svg",
  "/_next/(.*)",
];

function isPublicPath(pathname: string): boolean {
  for (const pattern of PUBLIC_ROUTES) {
    if (pattern.endsWith("(.*)")) {
      const prefix = pattern.slice(0, -4);
      if (pathname === prefix.replace(/\/$/, "") || pathname.startsWith(prefix)) {
        return true;
      }
    } else if (pathname === pattern) {
      return true;
    }
  }
  return false;
}

// We attempt to load Clerk's middleware at module init. If the package is
// absent (which it will be until `pnpm install @clerk/nextjs` runs), we
// export a no-op middleware that lets every request through — the mock
// auth provider in lib/auth.ts handles the dev case.
type MiddlewareFn = (req: NextRequest) => Promise<Response> | Response;

function buildMiddleware(): MiddlewareFn {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const clerk = require("@clerk/nextjs/server");
    if (typeof clerk.clerkMiddleware === "function") {
      return clerk.clerkMiddleware(
        (auth: { protect: () => void }, req: NextRequest) => {
          if (!isPublicPath(req.nextUrl.pathname)) {
            auth.protect();
          }
        },
      ) as MiddlewareFn;
    }
  } catch {
    /* Clerk not installed yet — fall through to pass-through. */
  }
  return (_req: NextRequest) => NextResponse.next();
}

export default buildMiddleware();

// Matcher: every route except Next internals + static assets. Mirrors the
// pattern Clerk recommends in its Next 15 quickstart.
export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.svg|.*\\.(?:png|jpg|jpeg|svg|webp|ico)).*)",
    "/(api|trpc)(.*)",
  ],
};
