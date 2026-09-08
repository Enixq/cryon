import type { Track } from "../types";

export interface SmartPlaylistMeta {
  id: string;
  title: string;
  description: string;
  accent: "violet" | "cyan" | "pink" | "rose" | "amber" | "emerald";
  tracks: Track[];
}

export function deriveSmartPlaylists(
  favorites: Track[],
  history: Array<{ track: Track; playedAt?: string }>,
): SmartPlaylistMeta[] {
  const favoriteTracks = favorites.slice(0, 6);

  // Счётчик и сам трек собираем за один проход. Раньше трек искали отдельным
  // history.find() на каждый уникальный id — линейный поиск внутри перебора,
  // то есть O(n²) на истории (а listHistory отдаёт до 300 записей).
  const frequency = new Map<string, { track: Track; count: number }>();
  for (const entry of history) {
    const existing = frequency.get(entry.track.id);
    if (existing) {
      existing.count += 1;
      continue;
    }
    frequency.set(entry.track.id, { track: entry.track, count: 1 });
  }

  const frequentTracks = Array.from(frequency.values())
    .sort((a, b) => b.count - a.count)
    .slice(0, 6)
    .map((item) => item.track);

  return [
    {
      id: "smart-favorites",
      title: "Мне нравится",
      description: "Треки, которые вы отметили сердцем",
      accent: "pink",
      tracks: favoriteTracks,
    },
    {
      id: "smart-frequent",
      title: "Часто слушаю",
      description: "Самые повторяющиеся треки из истории",
      accent: "cyan",
      tracks: frequentTracks,
    },
  ];
}
