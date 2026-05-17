import * as React from "react";

// Client-island wrapper for <ClerkProvider />. We resolve it at runtime so
// the build still succeeds when @clerk/nextjs isn't installed (mock mode).
let ClerkProvider: React.ComponentType<{ children: React.ReactNode }> | null =
  null;
if (process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY) {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    ClerkProvider = require("@clerk/nextjs").ClerkProvider;
  } catch {
    ClerkProvider = null;
  }
}

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const body = (
    <main className="min-h-screen flex items-center justify-center px-6 py-12">
      <div className="w-full max-w-sm space-y-6">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-muted-foreground)]">
            owera.ai
          </div>
          <div className="mt-1 font-mono text-lg font-semibold tracking-tight">
            Owera Agentic
          </div>
        </div>
        {children}
      </div>
    </main>
  );
  if (ClerkProvider) {
    return <ClerkProvider>{body}</ClerkProvider>;
  }
  return body;
}
