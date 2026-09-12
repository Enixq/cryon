import type { AccentColor } from "../types";

// Жанры для обзора на странице поиска. slug — часть URL (/genre/:slug).
//
// query — запасной поисковый запрос (используется, если у жанра нет seeds).
// seeds — реальные исполнители жанра. Клик по жанру собирает подборку из
// поиска ПО ЭТИМ ИСПОЛНИТЕЛЯМ, а не по названию жанра: запрос «русский рок»
// раньше находил треки, буквально названные «Русский рок», а не сам жанр.
// Порядок seeds значим — сборка чередует их по кругу для разнообразия.
export interface Genre {
  slug: string;
  title: string;
  query: string;
  seeds?: string[];
  accent: AccentColor;
}

export const genres: Genre[] = [
  {
    slug: "synthwave",
    title: "Синтвейв",
    query: "synthwave",
    seeds: ["The Midnight", "Gunship", "FM-84", "Carpenter Brut", "Perturbator", "Timecop1983", "The Weeknd Blinding Lights", "Kavinsky"],
    accent: "violet",
  },
  {
    slug: "russian-rock",
    title: "Русский рок",
    query: "русский рок",
    seeds: ["Кино", "ДДТ", "Сплин", "Би-2", "Ария", "Наутилус Помпилиус", "Алиса", "Земфира", "Мумий Тролль", "Король и Шут"],
    accent: "amber",
  },
  {
    slug: "electronic",
    title: "Электроника",
    query: "electronic dance",
    seeds: ["Daft Punk", "The Chemical Brothers", "deadmau5", "Justice", "Bonobo", "ODESZA", "Rüfüs Du Sol", "Flume"],
    accent: "cyan",
  },
  {
    slug: "indie",
    title: "Инди",
    query: "indie",
    seeds: ["Arctic Monkeys", "The Strokes", "Tame Impala", "Vampire Weekend", "Foals", "Two Door Cinema Club", "Phoenix", "Alt-J"],
    accent: "pink",
  },
  {
    slug: "hip-hop",
    title: "Хип-хоп",
    query: "hip hop",
    seeds: ["Kendrick Lamar", "Drake", "J. Cole", "Travis Scott", "Eminem", "Tyler, The Creator", "A$AP Rocky", "Kanye West"],
    accent: "orange",
  },
  {
    slug: "lofi",
    title: "Lo-fi",
    query: "lofi hip hop",
    seeds: ["Nujabes", "J Dilla", "Idealism", "Jinsang", "Tomppabeats", "Kupla", "L'indécis", "Philanthrope"],
    accent: "emerald",
  },
  {
    slug: "classical",
    title: "Классика",
    query: "classical music",
    seeds: ["Ludovico Einaudi", "Max Richter", "Ólafur Arnalds", "Hans Zimmer", "Yiruma", "Frédéric Chopin", "Claude Debussy", "Erik Satie"],
    accent: "slate",
  },
  {
    slug: "rock",
    title: "Рок",
    query: "rock",
    seeds: ["Queen", "Nirvana", "Foo Fighters", "Led Zeppelin", "Metallica", "Red Hot Chili Peppers", "AC/DC", "Radiohead"],
    accent: "rose",
  },
];

export function genreBySlug(slug: string | undefined): Genre | undefined {
  if (!slug) return undefined;
  return genres.find((g) => g.slug === slug);
}
