import { useQuery } from "@tanstack/react-query";
import { searchAll } from "../api/client";
import { accentStyle } from "./accents";
import type { Genre } from "../data/genres";

interface GenreCardProps {
  genre: Genre;
  onClick: () => void;
}

/**
 * Карточка жанра для обзора на странице поиска. Обложка — реальная картинка из
 * подборки треков жанра (задача 33): берётся коллаж из первых обложек, иначе
 * fallback на акцентный градиент с названием. Тот же queryKey, что и на
 * странице жанра, поэтому переход открывает подборку из кэша без повторной
 * загрузки.
 */
export function GenreCard({ genre, onClick }: GenreCardProps) {
  const { data } = useQuery({
    queryKey: ["genre", genre.query],
    queryFn: () => searchAll(genre.query),
    staleTime: 5 * 60 * 1000,
  });

  // Собираем до четырёх уникальных обложек для коллажа-плитки.
  const covers: string[] = [];
  for (const t of data ?? []) {
    if (t.coverUrl && !covers.includes(t.coverUrl)) covers.push(t.coverUrl);
    if (covers.length >= 4) break;
  }

  return (
    <button
      onClick={onClick}
      className="neon-frame group relative flex h-28 items-start overflow-hidden rounded-2xl p-4 text-left text-lg font-bold text-white"
      style={accentStyle(genre.accent)}
    >
      {covers.length > 0 && (
        <div
          className={
            covers.length >= 4
              ? "absolute inset-0 grid grid-cols-2 grid-rows-2"
              : "absolute inset-0"
          }
          aria-hidden
        >
          {(covers.length >= 4 ? covers.slice(0, 4) : covers.slice(0, 1)).map((src, i) => (
            <img
              key={`${src}-${i}`}
              src={src}
              alt=""
              loading="lazy"
              className="h-full w-full object-cover"
            />
          ))}
        </div>
      )}
      {/* Затемнение: сильнее поверх обложки (читаемость названия), легче поверх
          градиента-заглушки. */}
      <div
        className={
          covers.length > 0
            ? "absolute inset-0 bg-black/45 transition-colors group-hover:bg-black/30"
            : "absolute inset-0 bg-black/15 transition-colors group-hover:bg-black/5"
        }
      />
      <span className="relative">{genre.title}</span>
    </button>
  );
}
