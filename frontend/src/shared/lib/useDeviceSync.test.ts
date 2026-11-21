import { afterEach, describe, expect, it, vi } from "vitest";
import { usePlayerStore } from "../../store/playerStore";
import { startDeviceSync, ownsPlayback } from "./useDeviceSync";
import { setConfiguredBaseUrl } from "./httpBridge";

let stop: (() => void) | undefined;
afterEach(() => {
  stop?.();
  stop = undefined;
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  delete (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__;
});
describe("device ownership and transport", () => {
  it("retries next when a host heartbeat wins the revision race", async () => {
    vi.useFakeTimers();
    localStorage.setItem("cryon.device.id", "phone");
    setConfiguredBaseUrl("http://desktop:8899");
    (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__ = "http://127.0.0.1:12345";
    const tracks = [0, 1, 2].map((i) => ({
      id: `youtube:${i}`,
      title: `${i}`,
      artist: "Artist",
      source: "youtube" as const,
      duration: 180,
      accent: "violet" as const,
    }));
    let shared = {
      deviceId: "pc",
      revision: 1,
      queue: tracks,
      currentIndex: 0,
      isPlaying: true,
      progress: 22,
      volume: 68,
      muted: false,
      playbackDeviceId: "host",
    };
    let conflict = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        if (init?.method === "PUT") {
          if (conflict) {
            conflict = false;
            shared = { ...shared, revision: 2, progress: 23 };
            return { ok: false, status: 409 };
          }
          shared = { ...JSON.parse(String(init.body)), revision: shared.revision + 1 };
          return { ok: true, json: async () => ({ revision: shared.revision }) };
        }
        return { ok: true, json: async () => shared };
      }),
    );
    stop = startDeviceSync();
    await vi.advanceTimersByTimeAsync(0);
    usePlayerStore.getState().next();
    await vi.advanceTimersByTimeAsync(1000);
    expect(shared.currentIndex).toBe(1);
    expect(shared.playbackDeviceId).toBe("host");
  });
  it("assigns sound to one participant while both see a playing transport", () => {
    expect(ownsPlayback("host", "pc", false)).toBe(true);
    expect(ownsPlayback("host", "phone", true)).toBe(false);
    expect(ownsPlayback("phone", "pc", false)).toBe(false);
    expect(ownsPlayback("phone", "phone", true)).toBe(true);
  });
  it("adopts the host queue silently and sends next back to the host", async () => {
    vi.useFakeTimers();
    localStorage.setItem("cryon.device.id", "phone");
    setConfiguredBaseUrl("http://desktop:8899");
    (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__ = "http://127.0.0.1:12345";
    const tracks = [0, 1].map((i) => ({
      id: `youtube:${i}`,
      title: `${i}`,
      artist: "Artist",
      source: "youtube" as const,
      duration: 180,
      accent: "violet" as const,
    }));
    let shared = {
      deviceId: "pc",
      revision: 1,
      queue: tracks,
      currentIndex: 0,
      isPlaying: true,
      progress: 22,
      volume: 68,
      muted: false,
      playbackDeviceId: "host",
    };
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        shared = { ...JSON.parse(String(init.body)), revision: shared.revision + 1 };
        return { ok: true, json: async () => ({ revision: shared.revision }) };
      }
      return { ok: true, json: async () => shared };
    });
    vi.stubGlobal("fetch", fetchMock);
    stop = startDeviceSync();
    await vi.advanceTimersByTimeAsync(0);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
    expect(usePlayerStore.getState().playbackOnThisDevice).toBe(false);
    usePlayerStore.getState().next();
    await vi.advanceTimersByTimeAsync(0);
    expect(shared.currentIndex).toBe(1);
    expect(shared.playbackDeviceId).toBe("host");
  });
});
