import { ArrowLeft, Clock3, Play, ExternalLink } from "lucide-react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { accentStyle } from "../shared/ui/accents";
import { TrackRow } from "../shared/ui/TrackRow";
import { formatDuration, pluralWithCount } from "../shared/lib/format";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { listLocalTracks, listFavorites, getAlbumTracks, resolveAlbum, openExternal } from "../shared/api/client";
import { SOURCES } from "../shared/sources";
import { ArtistLink } from "../shared/ui/ArtistLink";
import type { SourceId, Track } from "../shared/types";

// Данные, которые страница-источник (поиск/исполнитель) передаёт через
// router state, чтобы открыть альбом с полным трек-листом.
interface AlbumNavState {
  service?: SourceId;
  albumId?: string; // id альбома в каталоге (для App.GetAlbumTracks)
  title: string;
  artist?: string;
  coverUrl?: string;
  externalUrl?: string;
  kind?: "album" | "single" | "ep";
  year?: number;
  accent?: Track["accent"];
  seedTracks?: Track[]; // треки, уже найденные поиском (показываем сразу)
}

const KIND_LABEL = { album: "Альбом", single: "Сингл", ep: "EP" } as const;

/**
 * Страница альбома. Открывается (не проигрывается сразу!) по клику из поиска
 * или страницы исполнителя. Полный трек-лист подгружается из каталога по
 * (service, albumId) через App.GetAlbumTracks; при навигации из «Медиатеки»
 * без state — собирается из локальных/избранных треков по ключу "альбом|артист".
 */
export function AlbumDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const nav = (location.state as { album?: AlbumNavState } | null)?.album;
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();

  // Каталожный трек-лист — грузим по (service, albumId), если они переданы.
  const hasCatalogRef = Boolean(nav?.service && nav?.albumId);
  const { data: catalogTracks, isFetching: catalogFetching } = useQuery({
    queryKey: ["albumTracks", nav?.service, nav?.albumId],
    queryFn: () => getAlbumTracks(nav!.service as SourceId, nav!.albumId as string),
    enabled: hasCatalogRef,
    staleTime: 5 * 60 * 1000,
  });

  // Альбом, открытый из поиска, не имеет id каталога — у него лишь несколько
  // найденных треков. Добираем полный трек-лист по «исполнитель + название»
  // (Yandex/YouTube на backend). Так «зайти в альбом → все треки» работает и из
  // поиска, а не только со страницы исполнителя.
  const searchDerived = Boolean(nav) && !hasCatalogRef;
  const canResolve = searchDerived && Boolean(nav?.title);
  const { data: resolvedTracks, isFetching: resolving } = useQuery({
    queryKey: ["resolveAlbum", nav?.artist, nav?.title],
    queryFn: () => resolveAlbum(nav?.artist ?? "", nav?.title ?? ""),
    enabled: canResolve,
    staleTime: 5 * 60 * 1000,
  });

  // Локальная библиотека — фолбэк для навигации из «Медиатеки» (без state).
  const { data: localTracks = [] } = useQuery({ queryKey: ["localTracks"], queryFn: listLocalTracks, enabled: !nav });
  const { data: favorites = [] } = useQuery({ queryKey: ["favorites"], queryFn: listFavorites, enabled: !nav });

  const decodedKey = id ? decodeURIComponent(id) : "";
  const [keyAlbum, keyArtist] = decodedKey.split("|");

  const albumName = nav?.title || keyAlbum || "Неизвестный альбом";
  const artistName = nav?.artist || keyArtist || "";

  // Итоговый список: каталог по id → добор по «исполнитель+название» → seed из
  // поиска → локальная библиотека.
  const localAlbumTracks = !nav && keyAlbum
    ? [...new Map([...localTracks, ...favorites].map((t) => [t.id, t])).values()].filter(
        (t) => t.album === keyAlbum && (!keyArtist || t.artist === keyArtist),
      )
    : [];
  const resolvedFull = Boolean(resolvedTracks && resolvedTracks.length > 0);
  const albumTracks: Track[] =
    catalogTracks && catalogTracks.length > 0
      ? catalogTracks
      : resolvedFull
        ? (resolvedTracks as Track[])
        : nav?.seedTracks && nav.seedTracks.length > 0
          ? nav.seedTracks
          : localAlbumTracks;

  const totalSeconds = albumTracks.reduce((sum, t) => sum + t.duration, 0);

  // Обложка: явная из state → встроенная обложка локального файла → из трека.
  const coverTrack = albumTracks[0];
  const localCover = useTrackCover(nav ? undefined : coverTrack);
  const coverUrl = nav?.coverUrl || localCover || coverTrack?.coverUrl || "";
  const accent = nav?.accent ?? coverTrack?.accent;

  // Полный трек-лист приходит из каталога, добора по названию или локальной
  // библиотеки. Если для альбома из поиска добор ещё идёт — показываем прогресс;
  // если добор ничего не дал — честно помечаем, что это лишь найденные треки.
  const catalogEmpty = hasCatalogRef && !catalogFetching && (catalogTracks?.length ?? 0) === 0;
  const stillResolving = canResolve && resolving && !resolvedFull;
  const seedOnly = searchDerived && !resolvedFull && !stillResolving;
  const showCount = !searchDerived || albumTracks.length > 0;
  const kindLabel = nav?.kind ? KIND_LABEL[nav.kind] : "Альбом";

  // Источники, из которых собран трек-лист — показываем пользователю, откуда
  // сейчас играет альбом (полезно на случай блокировок: плеер умеет
  // автоматически переключаться на другой источник той же композиции).
  const albumSources = [...new Set(albumTracks.map((t) => t.source))];

  return (
    <div className="flex flex-col gap-6 py-2">
      <button
        onClick={() => navigate(-1)}
        className="flex w-fit items-center gap-2 text-sm text-slate-400 transition-colors hover:text-white"
      >
        <ArrowLeft size={16} />
        Назад
      </button>

      {/* Шапка альбома */}
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <div
          className="relative h-40 w-40 shrink-0 overflow-hidden rounded-2xl shadow-2xl sm:h-48 sm:w-48"
          style={accent ? accentStyle(accent) : { background: "linear-gradient(135deg, #64748b, #94a3b8)" }}
        >
          {coverUrl && (
            <img
              src={coverUrl}
              alt={albumName}
              onError={(e) => (e.currentTarget.style.display = "none")}
              className="absolute inset-0 h-full w-full object-cover"
            />
          )}
        </div>
        <div className="flex min-w-0 flex-col items-center gap-3 sm:items-start">
          <span className="text-sm font-medium uppercase tracking-wider text-slate-400">{kindLabel}</span>
          <h1 className="max-w-full break-words text-3xl font-black text-white sm:text-5xl">{albumName}</h1>
          <div className="flex flex-wrap items-center justify-center gap-2 text-sm text-slate-400 sm:justify-start">
            {artistName && <ArtistLink name={artistName} className="font-medium text-white" />}
            {nav?.year ? (
              <>
                <span>•</span>
                <span>{nav.year}</span>
              </>
            ) : null}
            {showCount && (
              <>
                <span>•</span>
                <span>{pluralWithCount(albumTracks.length, "трек", "трека", "треков")}</span>
              </>
            )}
            {totalSeconds > 0 && (
              <>
                <span>•</span>
                <span>{formatDuration(totalSeconds)}</span>
              </>
            )}
          </div>
          {albumSources.length > 0 && (
            <div className="flex flex-wrap items-center justify-center gap-2 sm:justify-start">
              <span className="text-xs text-slate-500">Источник:</span>
              {albumSources.map((src) => (
                <span
                  key={src}
                  className="inline-flex items-center gap-1.5 rounded-full bg-white/8 px-2.5 py-1 text-xs font-medium text-slate-200"
                >
                  <span className="h-2 w-2 rounded-full" style={{ background: SOURCES[src]?.dot }} />
                  {SOURCES[src]?.name ?? src}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Действия */}
      <div className="flex flex-wrap items-center justify-center gap-3 sm:justify-start">
        {albumTracks.length > 0 && (
          <button
            onClick={() => playTrack(albumTracks[0], albumTracks)}
            className="flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105"
          >
            <Play size={20} fill="currentColor" />
            Слушать
          </button>
        )}
        {nav?.externalUrl && (
          <button
            onClick={() => void openExternal(nav.externalUrl as string)}
            className="flex items-center gap-2 rounded-full border border-white/12 px-5 py-3 text-sm font-medium text-slate-200 transition-colors hover:bg-white/5"
          >
            <ExternalLink size={16} />
            Открыть в источнике
          </button>
        )}
      </div>

      {/* Список треков */}
      <div className="flex flex-col">
        <div className="grid grid-cols-[40px_1fr_auto] items-center gap-4 border-b border-white/8 px-3 pb-2 text-xs uppercase tracking-wider text-slate-500">
          <span>#</span>
          <span>Название</span>
          <Clock3 size={14} className="justify-self-end" />
        </div>
        {(catalogFetching || stillResolving) && albumTracks.length === 0 ? (
          <div className="grid place-items-center py-20">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-white/20 border-t-[#a855f7]" />
          </div>
        ) : albumTracks.length > 0 ? (
          <>
            {stillResolving && (
              <p className="px-3 py-2 text-xs text-slate-500">Собираем полный трек-лист альбома…</p>
            )}
            {albumTracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={track}
                index={i}
                active={activeTrackId === track.id}
                playing={isPlaying}
                onPlay={(t) => playTrack(t, albumTracks)}
              />
            ))}
            {seedOnly && (
              <p className="mt-4 px-3 text-xs text-slate-500">
                Показаны треки, найденные в источниках: полный трек-лист этого альбома собрать не
                удалось. Попробуйте открыть релиз со страницы исполнителя или во внешнем сервисе.
              </p>
            )}
          </>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
            <div>
              <p>{catalogEmpty ? "Источник не вернул трек-лист этого альбома." : "Треки в этом альбоме не найдены."}</p>
              {nav?.externalUrl && (
                <button
                  onClick={() => void openExternal(nav.externalUrl as string)}
                  className="mt-4 inline-flex items-center gap-2 rounded-full bg-[#a855f7] px-5 py-2.5 text-sm font-semibold text-white transition-transform hover:scale-105"
                >
                  <ExternalLink size={16} />
                  Открыть в источнике
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default AlbumDetailPage;
