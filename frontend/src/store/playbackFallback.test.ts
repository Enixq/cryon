import { describe, expect, it } from "vitest";
import { PlaybackFallbackSession } from "./playbackFallback";
import type { Track } from "../shared/types";

const track = (id: string): Track => ({ id, source: id.split(":")[0] as Track["source"], title: "Song", artist: "Artist", duration: 180, accent: "violet" });

describe("PlaybackFallbackSession", () => {
  it("tries several versions once and preserves the position when a replacement fails immediately", () => {
    const session = new PlaybackFallbackSession();
    const first = track("youtube:first");
    const second = track("soundcloud:second");
    const third = track("youtube:third");
    expect(session.fail(first, 42)).toBe(true);
    session.select(second, first, "Совпадающая версия", 250);
    expect(session.fail(second, 0)).toBe(true);
    expect(session.anchor).toBe(first);
    expect([...session.attempted]).toEqual([first.id, second.id]);
    expect(session.resumePosition).toBe(42);
    session.select(third, second, "Совпадающая версия", 500);
    expect(session.started(third)?.position).toBe(42);
    expect(session.fail(first, 0)).toBe(false);
  });

  it("bounds retries and resets for another song", () => {
    const session = new PlaybackFallbackSession();
    for (let index = 0; index < 8; index++) expect(session.fail(track(`youtube:${index}`), 0)).toBe(true);
    expect(session.fail(track("youtube:ninth"), 0)).toBe(false);
    session.reset();
    expect(session.fail(track("youtube:ninth"), 0)).toBe(true);
  });
});
