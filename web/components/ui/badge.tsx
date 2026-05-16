import * as React from "react";
import { cn } from "./cn";

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** Optional explicit colour — defaults to muted. */
  tone?: string;
}

export function Badge({ className, tone, style, ...props }: BadgeProps) {
  const inlineStyle: React.CSSProperties = tone
    ? {
        backgroundColor: `${tone}1f`, // ~12% alpha
        color: tone,
        borderColor: `${tone}3d`,
        ...style,
      }
    : style ?? {};

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded border px-1.5 py-0.5",
        "text-xs font-medium uppercase tracking-wide font-mono",
        !tone &&
          "border-[var(--color-border)] bg-[var(--color-muted)] text-[var(--color-muted-foreground)]",
        className,
      )}
      style={inlineStyle}
      {...props}
    />
  );
}
