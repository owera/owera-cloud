"use client";

import * as React from "react";

interface CodeBlockProps {
  code: string;
  lang?: string;
  filename?: string;
}

export function CodeBlock({ code, lang, filename }: CodeBlockProps) {
  const [copied, setCopied] = React.useState(false);
  function onCopy() {
    void navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }
  return (
    <div className="border rounded-md bg-[var(--color-card)] border-[var(--color-border)] my-3 overflow-hidden">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-1.5 text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        <span>{filename ?? lang ?? "code"}</span>
        <button
          type="button"
          onClick={onCopy}
          className="hover:text-[var(--color-foreground)] focus-visible:outline-none focus-visible:text-[var(--color-foreground)]"
        >
          {copied ? "COPIED" : "COPY"}
        </button>
      </div>
      <pre className="px-3 py-2 overflow-x-auto text-xs leading-relaxed">
        <code className="font-mono text-[var(--color-foreground)]">{code}</code>
      </pre>
    </div>
  );
}
