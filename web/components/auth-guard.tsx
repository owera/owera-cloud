import * as React from "react";
import { redirect } from "next/navigation";
import { getCurrentUser } from "@/lib/auth";
import type { User } from "@/lib/types";

export interface AuthGuardProps {
  children: (user: User) => React.ReactNode;
  /** Where to redirect on miss. Defaults to "/". */
  redirectTo?: string;
}

/**
 * Server-side auth gate. Pages that require sign-in should wrap their content:
 *
 *   <AuthGuard>{(user) => <Dashboard user={user} />}</AuthGuard>
 *
 * Real auth wiring lives in `lib/auth.ts`; this component just enforces the gate.
 */
export async function AuthGuard({
  children,
  redirectTo = "/",
}: AuthGuardProps) {
  const user = await getCurrentUser();
  if (!user) redirect(redirectTo);
  return <>{children(user)}</>;
}
