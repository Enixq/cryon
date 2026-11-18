import { beforeEach, describe, expect, it } from "vitest";
import { usePlaybackDiagnosticsStore } from "./playbackDiagnosticsStore";

describe("playbackDiagnosticsStore", () => {
  beforeEach(() => {
    window.localStorage.clear();
    usePlaybackDiagnosticsStore.getState().clear();
  });

  it("keeps recent playback events on the device", () => {
    usePlaybackDiagnosticsStore.getState().record({
      kind: "fallback",
      trackId: "youtube:track",
      title: "Track",
      artist: "Artist",
      fromSource: "youtube",
      toSource: "soundcloud",
      positionSeconds: 42,
      message: "Выбрана доступная версия.",
      fallbackSearchMs: 1250,
    });

    const [event] = usePlaybackDiagnosticsStore.getState().items;
    expect(event.positionSeconds).toBe(42);
    expect(event.toSource).toBe("soundcloud");
    expect(event.fallbackSearchMs).toBe(1250);
    expect(JSON.parse(window.localStorage.getItem("cryon.playback-diagnostics.v1") || "[]")).toHaveLength(1);
  });
});
