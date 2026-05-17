import * as React from "react";
import Link from "next/link";

export const metadata = { title: "Sign up" };

export default async function SignUpPage() {
  const hasClerk = Boolean(process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY);
  if (hasClerk) {
    try {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const { SignUp } = require("@clerk/nextjs");
      return (
        <div className="space-y-4">
          <h1 className="font-mono text-base tracking-tight">SIGN UP</h1>
          <SignUp
            appearance={{ elements: { card: "shadow-none border border-[var(--color-border)]" } }}
            path="/sign-up"
            routing="path"
            signInUrl="/sign-in"
          />
        </div>
      );
    } catch {
      /* Fall through to placeholder. */
    }
  }
  return (
    <div className="space-y-4">
      <h1 className="font-mono text-base tracking-tight">SIGN UP</h1>
      <p className="text-sm text-[var(--color-muted-foreground)]">
        Self-service signup is not configured in this environment. Email{" "}
        <a className="underline" href="mailto:hello@owera.com">
          hello@owera.com
        </a>{" "}
        to provision a tenant.
      </p>
      <Link
        href="/sign-in"
        className="inline-flex h-9 px-3 items-center rounded-md text-sm font-medium border border-[var(--color-border)]"
      >
        Back to sign in
      </Link>
    </div>
  );
}
