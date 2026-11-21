import type { AccentColor } from "../types";
export interface Genre {
  slug: string;
  title: string;
  query: string;
  accent: AccentColor;
  seeds?: string[];
}
export const genres: Genre[] = [
  { slug: "rock", title: "Рок", query: "rock", accent: "rose" },
  { slug: "pop", title: "Поп", query: "pop", accent: "pink" },
  { slug: "hip-hop", title: "Хип-хоп", query: "hip hop", accent: "orange" },
  { slug: "electronic", title: "Электроника", query: "electronic", accent: "cyan" },
  { slug: "indie", title: "Инди и альтернатива", query: "alternative", accent: "violet" },
  { slug: "jazz", title: "Джаз", query: "jazz", accent: "amber" },
  { slug: "classical", title: "Классика", query: "classical", accent: "slate" },
  { slug: "rnb", title: "R&B", query: "rnb", accent: "emerald" },
  { slug: "dance", title: "Танцевальная", query: "dance", accent: "pink" },
  { slug: "metal", title: "Метал", query: "metal", accent: "rose" },
];
export function genreBySlug(slug: string | undefined): Genre | undefined {
  return genres.find((genre) => genre.slug === slug);
}
