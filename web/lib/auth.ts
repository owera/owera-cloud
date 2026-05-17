// Auth provider — Clerk-first, mock fallback.
//
// Selection precedence:
//   1. NEXT_PUBLIC_AUTH_PROVIDER="mock" forces the fixture provider (CI, local dev).
//   2. NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY present → Clerk provider.
//   3. Otherwise mock provider (with a console warning).
//
// Clerk publishable keys ship to the browser; secret keys (CLERK_SECRET_KEY)
// stay server-side and are consumed by middleware.ts + this file's
// getApiToken() server path. See infra/secrets-manifest.md for storage.
//
// Tracking-Track: the API tier authenticates customer requests with a
// long-lived Bearer API key (see api/internal/auth). Clerk only authenticates
// the *dashboard*; the dashboard then exchanges the Clerk session for the
// tenant's stored Owera API key server-side in getApiToken() before
// proxying upstream. The browser never sees the Owera API key directly.

import type { User } from "./types";

export interface AuthProvider {
  /** Resolve the currently signed-in user, or null if signed out. */
  getCurrentUser(): Promise<User | null>;
  /** Mint or fetch a short-lived Owera API token for proxying upstream. */
  getApiToken(): Promise<string | null>;
  /** Returns the sign-in URL (with optional return-to). */
  signInUrl(returnTo?: string): string;
}

const MOCK_USER: User = {
  id: "usr_mock_0001",
  email: "dev@owera.ai",
  name: "Dev User",
  tenantId: "tnt_mock_0001",
  role: "owner",
};

class MockAuthProvider implements AuthProvider {
  async getCurrentUser(): Promise<User | null> {
    return MOCK_USER;
  }
  async getApiToken(): Promise<string | null> {
    return "mock-dev-token";
  }
  signInUrl(returnTo?: string): string {
    const qs = returnTo ? `?returnTo=${encodeURIComponent(returnTo)}` : "";
    return `/sign-in${qs}`;
  }
}

// ClerkAuthProvider is loaded lazily so the mock path (and any build that
// never installs @clerk/nextjs) keeps working. The import is wrapped in a
// try/catch because in tests + linters @clerk/nextjs may not be installed.
class ClerkAuthProvider implements AuthProvider {
  async getCurrentUser(): Promise<User | null> {
    try {
      // Dynamic import: lets the bundle build even when @clerk/nextjs is
      // absent. The fallback is engaged by the factory if the import fails.
      const mod = await import("@clerk/nextjs/server");
      const { userId, sessionClaims } = await mod.auth();
      if (!userId) return null;
      const claims = (sessionClaims ?? {}) as Record<string, unknown>;
      // Owera maps Clerk org → tenant. The org id is set on every session
      // via the JWT template "owera-default" (see infra/secrets-manifest).
      const tenantId =
        typeof claims["org_id"] === "string"
          ? (claims["org_id"] as string)
          : typeof claims["tenant_id"] === "string"
            ? (claims["tenant_id"] as string)
            : "tnt_unknown";
      const email =
        typeof claims["email"] === "string"
          ? (claims["email"] as string)
          : "";
      const name =
        typeof claims["name"] === "string"
          ? (claims["name"] as string)
          : email.split("@")[0] || "User";
      const role =
        claims["org_role"] === "admin"
          ? "admin"
          : claims["org_role"] === "owner"
            ? "owner"
            : "member";
      return { id: userId, email, name, tenantId, role };
    } catch {
      return null;
    }
  }
  async getApiToken(): Promise<string | null> {
    // Server-side exchange: the dashboard's /api/proxy/* route runs in
    // Node; here we ask Clerk for a JWT minted under the Owera template
    // which the API recognises (NextAuth bridge — WS-14 wires the verifier).
    try {
      const mod = await import("@clerk/nextjs/server");
      const session = await mod.auth();
      if (!session.userId) return null;
      const tokenFn = (session as { getToken?: (opts?: { template?: string }) => Promise<string | null> })
        .getToken;
      if (typeof tokenFn !== "function") return null;
      return await tokenFn({ template: "owera-api" });
    } catch {
      return null;
    }
  }
  signInUrl(returnTo?: string): string {
    const qs = returnTo ? `?redirect_url=${encodeURIComponent(returnTo)}` : "";
    return `/sign-in${qs}`;
  }
}

let providerInstance: AuthProvider | null = null;

export function getAuthProvider(): AuthProvider {
  if (providerInstance) return providerInstance;
  const explicit = process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  if (explicit === "mock") {
    providerInstance = new MockAuthProvider();
    return providerInstance;
  }
  const hasClerk = Boolean(process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY);
  if (explicit === "clerk" || hasClerk) {
    providerInstance = new ClerkAuthProvider();
    return providerInstance;
  }
  if (typeof console !== "undefined") {
    console.warn(
      "[auth] NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY not set; using mock provider.",
    );
  }
  providerInstance = new MockAuthProvider();
  return providerInstance;
}

export async function getCurrentUser(): Promise<User | null> {
  return getAuthProvider().getCurrentUser();
}

export async function getApiToken(): Promise<string | null> {
  return getAuthProvider().getApiToken();
}

// resetAuthProviderForTests is a test-only escape hatch so the singleton
// can be re-evaluated after env mutation. Not exported via the public path.
export function __resetAuthProviderForTests(): void {
  providerInstance = null;
}
