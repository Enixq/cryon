import { describe, expect, it } from "vitest";
import { resolveAppAccent } from "./appTheme";
import { DEFAULT_ACCENT } from "./useCoverPalette";

describe("resolveAppAccent", () => {
  it("returns provided accent when it is present", () => {
    expect(resolveAppAccent("#123456")).toBe("#123456");
  });

  it("falls back to default accent when accent is empty", () => {
    expect(resolveAppAccent("   ")).toBe(DEFAULT_ACCENT);
    expect(resolveAppAccent(undefined)).toBe(DEFAULT_ACCENT);
  });
});
