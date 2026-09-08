import { describe, expect, it } from "vitest";
import { deriveSmartPlaylists } from "./smartPlaylists";
import type { HistoryEntry, Track } from "../types";

const makeTrack = (id: string, title: string): Track => ({
  id,
  title,
  artist: "Test Artist",
  source: "youtube",
  duration: 180,
  accent: "violet",
});

describe("deriveSmartPlaylists", () => {
  it("creates smart playlists from favorites and history", () => {
    const favorites = [makeTrack("f1", "Favorite Track")];
    const history = [
      { track: makeTrack("f1", "Favorite Track"), playedAt: "2026-07-01T12:00:00Z" },
      { track: makeTrack("f1", "Favorite Track"), playedAt: "2026-07-01T13:00:00Z" },
      { track: makeTrack("h2", "Frequent Track"), playedAt: "2026-07-01T14:00:00Z" },
    ];

    const playlists = deriveSmartPlaylists(favorites, history satisfies HistoryEntry[]);

    expect(playlists[0]).toMatchObject({ id: "smart-favorites", title: "Мне нравится" });
    expect(playlists[1]).toMatchObject({ id: "smart-frequent", title: "Часто слушаю" });
  });
});
