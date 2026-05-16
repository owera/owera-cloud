// Small formatting helpers. Keep these dependency-free; if we need a real
// i18n library it should be wired here and only here.

/** Format USD cents as "$1,234.56". */
export function formatCents(cents: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}

/** Format an integer count with thousands separators. */
export function formatCount(n: number): string {
  return new Intl.NumberFormat("en-US").format(n);
}

const RTF = new Intl.RelativeTimeFormat("en-US", { numeric: "auto" });

interface Unit {
  unit: Intl.RelativeTimeFormatUnit;
  seconds: number;
}

const UNITS: ReadonlyArray<Unit> = [
  { unit: "year", seconds: 31536000 },
  { unit: "month", seconds: 2592000 },
  { unit: "week", seconds: 604800 },
  { unit: "day", seconds: 86400 },
  { unit: "hour", seconds: 3600 },
  { unit: "minute", seconds: 60 },
  { unit: "second", seconds: 1 },
];

/** Relative time string. Pass an ISO-8601 timestamp. */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const diffSec = Math.round((then - now.getTime()) / 1000);
  const absSec = Math.abs(diffSec);
  for (const u of UNITS) {
    if (absSec >= u.seconds || u.unit === "second") {
      const value = Math.round(diffSec / u.seconds);
      return RTF.format(value, u.unit);
    }
  }
  return iso;
}

/** Short absolute timestamp e.g. "2026-05-16 14:23 UTC". */
export function shortTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ` +
    `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())} UTC`
  );
}

/** Format an elapsed duration between two timestamps. */
export function duration(startIso: string, endIso: string | null): string {
  const start = new Date(startIso).getTime();
  const end = endIso ? new Date(endIso).getTime() : Date.now();
  if (Number.isNaN(start) || Number.isNaN(end)) return "—";
  const ms = Math.max(0, end - start);
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ${sec % 60}s`;
  const hr = Math.floor(min / 60);
  return `${hr}h ${min % 60}m`;
}

/** Compose class names without pulling tailwind-merge here — keep small. */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
