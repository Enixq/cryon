import type { Track } from "../types";
import { deriveSmartPlaylists } from "./smartPlaylists";
import type { SmartPlaylistMeta } from "./smartPlaylists";

export { deriveSmartPlaylists };

function uniqueTracks(tracks: Track[]): Track[] {
  const seen = new Set<string>();
  const result: Track[] = [];
  for (const track of tracks) {
    if (seen.has(track.id)) continue;
    seen.add(track.id);
    result.push(track);
  }
  return result;
}

function parsePlayedAt(value: string | undefined): number {
  if (!value) return 0;
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : 0;
}

export function deriveRecommendations(
  favorites: Track[],
  history: Array<{ track: Track; playedAt?: string }>,
  localTracks: Track[],
): SmartPlaylistMeta[] {
  const favoriteIds = new Set(favorites.map((track) => track.id));
  const recentIds = new Set<string>();

  const historyRows = history.map((entry) => ({
    track: entry.track,
    playedAt: parsePlayedAt(entry.playedAt),
  }));

  const frequency = new Map<string, { track: Track; count: number; lastPlayed: number }>();
  for (const row of historyRows) {
    const existing = frequency.get(row.track.id);
    if (!existing) {
      frequency.set(row.track.id, {
        track: row.track,
        count: 1,
        lastPlayed: row.playedAt,
      });
      continue;
    }
    existing.count += 1;
    if (row.playedAt > existing.lastPlayed) {
      existing.lastPlayed = row.playedAt;
    }
  }

  const frequentTracks = Array.from(frequency.values())
    .sort((a, b) => {
      if (b.count !== a.count) return b.count - a.count;
      return b.lastPlayed - a.lastPlayed;
    })
    .map((item) => item.track);

  const recentTracks = historyRows
    .sort((a, b) => b.playedAt - a.playedAt)
    .map((item) => item.track)
    .filter((track) => {
      if (recentIds.has(track.id)) return false;
      recentIds.add(track.id);
      return true;
    });

  const discoveryCandidates = [
    ...recentTracks.filter((track) => !favoriteIds.has(track.id)),
    ...localTracks.filter((track) => !favoriteIds.has(track.id)),
    ...frequentTracks.filter((track) => !favoriteIds.has(track.id)),
  ];

  const discoveryTracks = uniqueTracks(discoveryCandidates).slice(0, 8);
  const favoriteTracks = uniqueTracks(favorites).slice(0, 6);
  const mixTracks = uniqueTracks([...favoriteTracks, ...frequentTracks, ...recentTracks]).slice(0, 10);

  return [
    {
      id: "rec-mix-day",
      title: "Микс дня",
      description: "Треки на сегодня, собранные из любимых и часто слушаемых",
      accent: "violet",
      tracks: mixTracks,
    },
    {
      id: "rec-discoveries-week",
      title: "Открытия недели",
      description: "Новые находки и интересные треки, основанные на вашей истории",
      accent: "cyan",
      tracks: discoveryTracks,
    },
  ];
}
