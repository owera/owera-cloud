import * as React from "react";
import Link from "next/link";

// Clerk renders its own <SignIn /> widget when the package is installed.
// Until it is, we ship a placeholder so the route is reachable from
// middleware redirects without a 404.

export const metadata = { title: "Sign in" };

export default async function SignInPage() {
  const hasClerk = Boolean(process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY);
  if (hasClerk) {
    try {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const { SignIn } = require("@clerk/nextjs");
      return (
        <div className="space-y-4">
          <h1 className="font-mono text-base tracking-tight">SIGN IN</h1>
          <SignIn
            appearance={{ elements: { card: "shadow-none border border-[var(--color-border)]" } }}
            path="/sign-in"
            routing="path"
            signUpUrl="/sign-up"
          />
        </div>
      );
    } catch {
      /* Fall through to placeholder. */
    }
  }
  return (
    <div className="space-y-4">
      <h1 className="font-mono text-base tracking-tight">SIGN IN</h1>
      <p className="text-sm text-[var(--color-muted-foreground)]">
        Auth provider is not configured in this environment. Set{" "}
        <code className="font-mono">NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY</code> to
        enable real sign-in, or proceed with the mock dev user.
      </p>
      <Link
        href="/dashboard"
        className="inline-flex h-9 px-3 items-center rounded-md text-sm font-medium bg-[var(--color-primary)] text-[var(--color-primary-foreground)]"
      >
        Continue as dev user
      </Link>
    </div>
  );
}
