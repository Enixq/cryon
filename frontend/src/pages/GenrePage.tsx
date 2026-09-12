import { ArrowLeft, Play, Music2 } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { searchAll } from "../shared/api/client";
import { genreBySlug, type Genre } from "../shared/data/genres";
import type { Track } from "../shared/types";
import { TrackRow } from "../shared/ui/TrackRow";
import { Cover } from "../shared/ui/Cover";
import { accentStyle } from "../shared/ui/accents";
import { pluralWithCount } from "../shared/lib/format";

// Сколько исполнителей-семян реально опрашиваем и сколько треков берём.
// Ограничения держат нагрузку на встроенный (мобильный) backend разумной:
// каждый seed — это полный мультиисточниковый поиск.
const MAX_SEEDS = 6;
const MAX_PER_SEED = 8;
const MAX_TOTAL = 60;

// Ключ дедупа: одна и та же песня из YouTube и SoundCloud имеет разные id,
// поэтому схлопываем по «исполнитель|название», а не по id.
function dedupeKey(t: Track): string {
  return `${t.artist}|${t.title}`.toLowerCase().replace(/\s+/g, " ").trim();
}

/**
 * Собирает подборку жанра по РЕАЛЬНЫМ исполнителям (genre.seeds), а не по
 * названию жанра. Раньше поиск шёл по строке вида «русский рок» и находил
 * треки, буквально так названные, а не сам жанр. Теперь опрашиваем несколько
 * артистов жанра и чередуем их треки по кругу — подборка перемешана и
 * представляет жанр, а не одного исполнителя.
 */
async function collectGenreTracks(genre: Genre): Promise<Track[]> {
  const seeds = genre.seeds && genre.seeds.length > 0 ? genre.seeds : [genre.query];
  const picked = seeds.slice(0, MAX_SEEDS);

  // allSettled: неудача по одному исполнителю (таймаут, VPN) не должна ронять
  // всю подборку — просто берём то, что нашлось у остальных.
  const settled = await Promise.allSettled(picked.map((s) => searchAll(s)));
  const buckets = settled.map((r) => (r.status === "fulfilled" ? r.value : []));

  const seen = new Set<string>();
  const out: Track[] = [];
  for (let round = 0; round < MAX_PER_SEED && out.length < MAX_TOTAL; round++) {
    for (const bucket of buckets) {
      const track = bucket[round];
      if (!track) continue;
      const key = dedupeKey(track);
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(track);
      if (out.length >= MAX_TOTAL) break;
    }
  }
  return out;
}

/**
 * Страница жанра: реальная подборка треков по жанру, оформленная как рабочий
 * плейлист. Треки ищутся в источниках через backend (App.SearchAll), их можно
 * воспроизвести целиком (play-all) или по одному. Обложка — картинка первого
 * трека подборки, иначе градиент-заглушка по акценту жанра (задача 22/33).
 */
export function GenrePage() {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const genre = genreBySlug(slug);

  const { data: tracks, isFetching, isError } = useQuery({
    queryKey: ["genre", genre?.slug],
    queryFn: () => collectGenreTracks(genre!),
    enabled: Boolean(genre),
    staleTime: 5 * 60 * 1000,
  });

  if (!genre) {
    return (
      <div className="flex flex-col gap-6 py-2">
        <BackButton onClick={() => navigate(-1)} />
        <div className="rounded-2xl border border-white/8 bg-white/5 p-8 text-center text-slate-400">
          Жанр не найден.
        </div>
      </div>
    );
  }

  const list = tracks ?? [];
  const coverSrc = list.find((t) => t.coverUrl)?.coverUrl;

  const playAll = () => {
    if (list.length > 0) playTrack(list[0], list);
  };

  return (
    <div className="flex flex-col gap-6 py-2">
      <BackButton onClick={() => navigate(-1)} />

      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <Cover
          accent={genre.accent}
          src={coverSrc}
          alt={genre.title}
          className="h-40 w-40 shrink-0 shadow-2xl sm:h-48 sm:w-48"
          iconSize={64}
          neon
        />
        <div className="flex min-w-0 flex-col items-center sm:items-start">
          <p className="text-sm font-medium uppercase tracking-wide text-slate-400">Жанр</p>
          <h1 className="mt-1 max-w-full truncate text-3xl font-black text-white sm:text-5xl">{genre.title}</h1>
          <p className="mt-2 text-sm text-slate-400">
            {isFetching
              ? "Собираем подборку…"
              : pluralWithCount(list.length, "трек", "трека", "треков")}
          </p>
          <button
            onClick={playAll}
            disabled={list.length === 0}
            className="neon-frame mt-5 flex items-center gap-2 rounded-full px-6 py-2.5 text-sm font-semibold text-white transition-transform hover:scale-[1.02] disabled:cursor-not-allowed disabled:opacity-40"
            style={accentStyle(genre.accent)}
          >
            <Play size={18} fill="currentColor" />
            Слушать
          </button>
        </div>
      </div>

      {isError ? (
        <div className="rounded-2xl border border-white/8 bg-white/5 p-8 text-center text-slate-400">
          Не удалось загрузить подборку. Попробуйте позже.
        </div>
      ) : list.length === 0 && !isFetching ? (
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
          <div>
            <Music2 size={40} className="mx-auto mb-3 opacity-40" />
            <p>В этом жанре пока ничего не нашлось.</p>
          </div>
        </div>
      ) : (
        <section className="flex flex-col">
          {list.map((track, i) => (
            <TrackRow
              key={track.id}
              track={track}
              index={i}
              active={activeTrackId === track.id}
              playing={isPlaying}
              onPlay={(t) => playTrack(t, list)}
              showAlbum
            />
          ))}
        </section>
      )}
    </div>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex w-fit items-center gap-2 text-sm text-slate-400 transition-colors hover:text-white"
    >
      <ArrowLeft size={16} />
      Назад
    </button>
  );
}

export default GenrePage;
