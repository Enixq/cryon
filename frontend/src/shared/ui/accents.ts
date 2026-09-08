import type { CSSProperties } from "react";
import type { AccentColor } from "../types";

/** Градиенты обложек и карточек по акцентному цвету. */
export const ACCENT_GRADIENT: Record<AccentColor, string> = {
  violet: "linear-gradient(135deg, #7c3aed 0%, #4c1d95 100%)",
  cyan: "linear-gradient(135deg, #06b6d4 0%, #0e3a5c 100%)",
  pink: "linear-gradient(135deg, #ec4899 0%, #7a1e52 100%)",
  orange: "linear-gradient(135deg, #f97316 0%, #7c2d12 100%)",
  rose: "linear-gradient(135deg, #f43f5e 0%, #7a1029 100%)",
  amber: "linear-gradient(135deg, #f59e0b 0%, #78350f 100%)",
  slate: "linear-gradient(135deg, #64748b 0%, #1e293b 100%)",
  emerald: "linear-gradient(135deg, #10b981 0%, #064e3b 100%)",
};

export function accentStyle(accent: AccentColor): CSSProperties {
  return { background: ACCENT_GRADIENT[accent] };
}
