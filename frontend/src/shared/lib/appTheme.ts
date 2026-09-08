import type { CSSProperties } from "react";
import { DEFAULT_ACCENT } from "./useCoverPalette";

export function resolveAppAccent(accent: string | undefined, fallback = DEFAULT_ACCENT): string {
  return accent && accent.trim() ? accent : fallback;
}

export function getAccentCssVariables(accent: string | undefined, fallback = DEFAULT_ACCENT): CSSProperties {
  return { "--app-accent": resolveAppAccent(accent, fallback) } as CSSProperties;
}
