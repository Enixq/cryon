import { describe, it, expect } from "vitest";
import { formatDuration, clamp, plural, pluralWithCount } from "./format";

describe("formatDuration", () => {
  it("форматирует секунды как m:ss", () => {
    expect(formatDuration(0)).toBe("0:00");
    expect(formatDuration(5)).toBe("0:05");
    expect(formatDuration(65)).toBe("1:05");
    expect(formatDuration(599)).toBe("9:59");
  });

  it("добавляет часы при длительности > 1ч", () => {
    expect(formatDuration(3600)).toBe("1:00:00");
    expect(formatDuration(3661)).toBe("1:01:01");
  });

  it("отсекает отрицательные и дробные значения", () => {
    expect(formatDuration(-10)).toBe("0:00");
    expect(formatDuration(65.9)).toBe("1:05");
  });
});

describe("clamp", () => {
  it("ограничивает значение диапазоном", () => {
    expect(clamp(5, 0, 10)).toBe(5);
    expect(clamp(-1, 0, 10)).toBe(0);
    expect(clamp(11, 0, 10)).toBe(10);
  });
});

describe("plural (русское склонение)", () => {
  it("выбирает форму 'один'", () => {
    expect(plural(1, "трек", "трека", "треков")).toBe("трек");
    expect(plural(21, "трек", "трека", "треков")).toBe("трек");
  });

  it("выбирает форму 'несколько'", () => {
    expect(plural(2, "трек", "трека", "треков")).toBe("трека");
    expect(plural(4, "трек", "трека", "треков")).toBe("трека");
    expect(plural(23, "трек", "трека", "треков")).toBe("трека");
  });

  it("выбирает форму 'много'", () => {
    expect(plural(0, "трек", "трека", "треков")).toBe("треков");
    expect(plural(5, "трек", "трека", "треков")).toBe("треков");
    expect(plural(11, "трек", "трека", "треков")).toBe("треков");
    expect(plural(12, "трек", "трека", "треков")).toBe("треков");
    expect(plural(100, "трек", "трека", "треков")).toBe("треков");
  });

  it("pluralWithCount добавляет число", () => {
    expect(pluralWithCount(1, "трек", "трека", "треков")).toBe("1 трек");
    expect(pluralWithCount(5, "трек", "трека", "треков")).toBe("5 треков");
  });
});
