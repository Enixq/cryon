import { Play } from "lucide-react";
import { accentStyle } from "./accents";
import type { AccentColor, Track } from "../types";

interface SmartPlaylistLike {
  id: string;
  title: string;
  description: string;
  accent: AccentColor;
  tracks: Track[];
}

/**
 * Карточка умной подборки с обложкой-задником.
 *
 * Поведение (задача из скриншотов 1 и 4): клик по всей карточке ОТКРЫВАЕТ
 * экран подборки со списком треков (onOpen), а не запускает воспроизведение —
 * играет только отдельная круглая кнопка (onPlay). Раньше вся карточка была
 * одной кнопкой play, из-за чего список треков было не открыть.
 *
 * Задник — коллаж из обложек первых треков (как в Spotify), поверх него
 * затемняющий градиент для читаемости текста. Если обложек нет — акцентный
 * градиент-заглушка. Так карточки выглядят как настоящие рекомендации с
 * «фоновой картинкой», а не плоские цветные плашки.
 *
 * Разметка без вложенных <button>: прозрачная кнопка на всю карточку ловит
 * «открыть», содержимое над ней pointer-events-none пропускает клики насквозь,
 * а play-кнопка их перехватывает (pointer-events-auto).
 */
export function SmartPlaylistCard({
  playlist,
  onOpen,
  onPlay,
}: {
  playlist: SmartPlaylistLike;
  onOpen: () => void;
  onPlay: () => void;
}) {
  const covers = playlist.tracks
    .map((t) => t.coverUrl)
    .filter((url): url is string => Boolean(url))
    .slice(0, 4);

  return (
    <div className="neon-card group relative flex aspect-[16/10] flex-col justify-end overflow-hidden rounded-2xl p-4 text-left">
      {/* Задник: коллаж обложек либо акцентная заглушка */}
      {covers.length >= 4 ? (
        <div className="absolute inset-0 grid grid-cols-2 grid-rows-2">
          {covers.map((url, i) => (
            <img key={i} src={url} alt="" loading="lazy" className="h-full w-full object-cover" />
          ))}
        </div>
      ) : covers.length > 0 ? (
        <img src={covers[0]} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
      ) : (
        <div className="absolute inset-0" style={accentStyle(playlist.accent)} />
      )}

      {/* Затемнение снизу вверх — чтобы текст читался поверх любой обложки */}
      <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/45 to-black/10" />

      {/* Прозрачная кнопка на всю карточку: открыть экран подборки */}
      <button onClick={onOpen} className="absolute inset-0 z-0" aria-label={`Открыть подборку «${playlist.title}»`} />

      {/* Кнопка воспроизведения */}
      <button
        onClick={onPlay}
        disabled={playlist.tracks.length === 0}
        className="pointer-events-auto absolute right-3 top-3 z-10 grid h-10 w-10 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-opacity group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-0"
        aria-label={`Воспроизвести подборку «${playlist.title}»`}
      >
        <Play size={17} fill="currentColor" className="ml-0.5" />
      </button>

      {/* Текст поверх задника */}
      <div className="pointer-events-none relative z-10">
        <strong className="block truncate text-base font-semibold text-white drop-shadow">{playlist.title}</strong>
        <p className="mt-1 line-clamp-2 text-xs text-slate-300 drop-shadow">{playlist.description}</p>
      </div>
    </div>
  );
}
