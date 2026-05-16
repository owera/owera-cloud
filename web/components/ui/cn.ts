// Local class-name helper for components/ui primitives.
// Wraps clsx + tailwind-merge so component variants can compose cleanly.
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
