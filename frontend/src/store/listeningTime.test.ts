import { describe, expect, it } from "vitest";
import { consumeListening, observeListening } from "./listeningTime";

describe("observed listening", () => {
  it("does not turn seeking to the end or paused time into a completion", () => {
    observeListening("seek", 0, true, 0);
    observeListening("seek", 1, true, 1000);
    observeListening("seek", 175, true, 1100);
    observeListening("seek", 176, true, 2100);
    observeListening("seek", 177, false, 3100);
    expect(consumeListening("seek")).toBe(2000);
    expect(consumeListening("seek")).toBe(0);
  });
  it("ignores stale polling gaps and counts normal forward progress", () => {
    observeListening("gap", 0, true, 0);
    observeListening("gap", 100, true, 100000);
    observeListening("gap", 101, true, 101000);
    expect(consumeListening("gap")).toBe(1000);
  });
});
