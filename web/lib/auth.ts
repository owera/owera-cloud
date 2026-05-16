// Auth provider stub.
//
// This scaffold ships a MockAuthProvider that returns a fake signed-in user.
// A follow-on PR wires this to Clerk or WorkOS based on the
// NEXT_PUBLIC_AUTH_PROVIDER env var (allowed: "mock" | "clerk" | "workos").

import type { User } from "./types";

export interface AuthProvider {
  /** Resolve the currently signed-in user, or null if signed out. */
  getCurrentUser(): Promise<User | null>;
  /** Mint or fetch a short-lived API token for the upstream Owera API. */
  getApiToken(): Promise<string | null>;
  /** Returns the marketing redirect URL when no user is present. */
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
    // A static dev token — the upstream API recognises this in mock mode.
    return "mock-dev-token";
  }
  signInUrl(returnTo?: string): string {
    const qs = returnTo ? `?returnTo=${encodeURIComponent(returnTo)}` : "";
    return `/sign-in${qs}`;
  }
}

let providerInstance: AuthProvider | null = null;

export function getAuthProvider(): AuthProvider {
  if (providerInstance) return providerInstance;
  const kind = process.env.NEXT_PUBLIC_AUTH_PROVIDER ?? "mock";
  switch (kind) {
    case "clerk":
    case "workos":
      // Not wired yet — fall through to mock with a console hint.
      if (typeof console !== "undefined") {
        console.warn(
          `[auth] NEXT_PUBLIC_AUTH_PROVIDER=${kind} is not wired yet; using mock provider.`,
        );
      }
      providerInstance = new MockAuthProvider();
      return providerInstance;
    case "mock":
    default:
      providerInstance = new MockAuthProvider();
      return providerInstance;
  }
}

export async function getCurrentUser(): Promise<User | null> {
  return getAuthProvider().getCurrentUser();
}

export async function getApiToken(): Promise<string | null> {
  return getAuthProvider().getApiToken();
}
