import type { AccentColor } from "../types";

// Жанры для обзора на странице поиска. slug — часть URL (/genre/:slug),
// query — реальный поисковый запрос в источники (клик открывает подборку
// треков жанра как рабочий плейлист, а не заглушку).
export interface Genre {
  slug: string;
  title: string;
  query: string;
  accent: AccentColor;
}

export const genres: Genre[] = [
  { slug: "synthwave", title: "Синтвейв", query: "synthwave", accent: "violet" },
  { slug: "russian-rock", title: "Русский рок", query: "русский рок", accent: "amber" },
  { slug: "electronic", title: "Электроника", query: "electronic dance", accent: "cyan" },
  { slug: "indie", title: "Инди", query: "indie", accent: "pink" },
  { slug: "hip-hop", title: "Хип-хоп", query: "hip hop", accent: "orange" },
  { slug: "lofi", title: "Lo-fi", query: "lofi hip hop", accent: "emerald" },
  { slug: "classical", title: "Классика", query: "classical music", accent: "slate" },
  { slug: "rock", title: "Рок", query: "rock", accent: "rose" },
];

export function genreBySlug(slug: string | undefined): Genre | undefined {
  if (!slug) return undefined;
  return genres.find((g) => g.slug === slug);
}
