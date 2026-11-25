import { afterEach, describe, expect, it, vi } from "vitest";
import {
  canUseDJ,
  djRadio,
  getDJStatus,
  reportDJEvent,
  resetDJSession,
  saveDJConfig,
} from "./intelligence";
import { mapTrack } from "./mappers";
import type { domain } from "../../../wailsjs/go/models";

afterEach(() => {
  resetDJSession();
  vi.unstubAllGlobals();
  delete (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__;
});

describe("AI DJ API", () => {
  it("keeps the default runtime disabled", () => {
    expect(canUseDJ()).toBe(false);
  });
  it("tracks plan provenance and never reads a saved key", async () => {
    (window as Window & { __CRYON_BASE__?: string }).__CRYON_BASE__ = "http://localhost:9999";
    const raw = {
      id: "new",
      service: "soundcloud",
      title: "New",
      artists: ["Artist"],
      playableKind: "stream",
      djPlanId: "pool",
    } as domain.Track;
    const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
      const body = url.endsWith("/radio")
        ? { sessionId: "session", tracks: [raw] }
        : { enabled: true, configured: true, provider: "openai", model: "test", dailySegments: 10 };
      if (url.endsWith("/feedback"))
        expect(JSON.parse(init!.body as string)).toMatchObject({
          session_id: "session",
          plan_id: "pool",
          track_id: "soundcloud:new",
          kind: "like",
        });
      return new Response(JSON.stringify(body), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher);
    expect((await getDJStatus()).configured).toBe(true);
    const tracks = await djRadio(null, mapTrack(raw), [], 10);
    expect(tracks).toHaveLength(1);
    reportDJEvent(tracks![0], "like");
    await Promise.resolve();
    expect(fetcher.mock.calls.some(([url]) => url.endsWith("/feedback"))).toBe(true);
    await saveDJConfig({
      enabled: false,
      provider: "openai",
      model: "test",
      baseUrl: "",
      apiKey: "",
      dailySegments: 10,
    });
    expect(fetcher.mock.calls.every(([url]) => !url.includes("settings"))).toBe(true);
  });
});
