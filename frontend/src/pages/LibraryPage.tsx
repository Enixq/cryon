import { FolderOpen, Library as LibraryIcon } from "lucide-react";
import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { TrackRow } from "../shared/ui/TrackRow";
import { PlaylistCard } from "../shared/ui/PlaylistCard";
import { Cover } from "../shared/ui/Cover";
import { PageHeader } from "../shared/ui/PageHeader";
import { TabBar, type TabItem } from "../shared/ui/TabBar";
import { ArtistLink } from "../shared/ui/ArtistLink";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { pluralWithCount } from "../shared/lib/format";
import { listLocalTracks, listFavorites, listPlaylists } from "../shared/api/client";
import type { Track } from "../shared/types";

type Tab = "tracks" | "albums" | "artists" | "playlists";

/**
 * Обложка по треку-образцу: использует хук useTrackCover, чтобы подгрузить
 * встроенную обложку локального файла (data-URL), если coverUrl пуст.
 * Нельзя вызывать хук в цикле map — поэтому отдельный компонент.
 *
 * NOTE: Доработано koda — обложки альбомов/артистов в библиотеке теперь
 * берутся через useTrackCover, а не напрямую из coverUrl (иначе локальные
 * треки показывали градиентную заглушку вместо реальной обложки).
 */
function CoverFromTrack({ track, alt, className, rounded, iconSize, neon }: {
  track: Track;
  alt: string;
  className?: string;
  rounded?: string;
  iconSize?: number;
  neon?: boolean;
}) {
  const coverUrl = useTrackCover(track);
  return (
    <Cover
      accent={track.accent}
      src={coverUrl}
      alt={alt}
      className={className}
      rounded={rounded}
      iconSize={iconSize}
      neon={neon}
    />
  );
}

// TypeBadge — лаконичная метка типа карточки поверх обложки (как на макете
// библиотеки): «Альбом», «Артист», «Плейлист».
function TypeBadge({ label }: { label: string }) {
  return (
    <span className="absolute left-2 top-2 rounded-full bg-black/55 px-2 py-0.5 text-[11px] font-medium text-white backdrop-blur-sm">
      {label}
    </span>
  );
}

// NOTE: доработано koda — LibraryPage ранее была полностью на моках
// (albums/artists/tracks/userPlaylists из mocks/data). Теперь берёт реальные
// треки из listLocalTracks + listFavorites и группирует по альбому/артисту.
export function LibraryPage() {
  const [tab, setTab] = useState<Tab>("tracks");
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const navigate = useNavigate();

  const { data: localTracks = [] } = useQuery({
    queryKey: ["localTracks"],
    queryFn: listLocalTracks,
  });

  const { data: favorites = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
  });

  const { data: playlists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
  });

  // Объединяем локальные треки и избранное, дедуп по id.
  const allTracks = useMemo(() => {
    const seen = new Set<string>();
    const result: Track[] = [];
    for (const t of [...localTracks, ...favorites]) {
      if (seen.has(t.id)) continue;
      seen.add(t.id);
      result.push(t);
    }
    return result;
  }, [localTracks, favorites]);

  // Группируем треки по альбому.
  const albumsMap = useMemo(() => {
    const map = new Map<string, Track[]>();
    for (const t of allTracks) {
      if (!t.album) continue;
      const key = `${t.album}|${t.artist}`;
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(t);
    }
    return map;
  }, [allTracks]);

  // Группируем треки по артисту.
  const artistsMap = useMemo(() => {
    const map = new Map<string, Track[]>();
    for (const t of allTracks) {
      if (!t.artist) continue;
      if (!map.has(t.artist)) map.set(t.artist, []);
      map.get(t.artist)!.push(t);
    }
    return map;
  }, [allTracks]);

  const tabs: TabItem<Tab>[] = [
    { id: "tracks", label: "Треки", count: allTracks.length },
    { id: "albums", label: "Альбомы", count: albumsMap.size },
    { id: "artists", label: "Исполнители", count: artistsMap.size },
    { id: "playlists", label: "Плейлисты", count: playlists.length },
  ];

  const handleAddFolder = () => navigate("/settings");

  return (
    <div className="flex flex-col gap-6 py-2">
      <PageHeader
        title="Библиотека"
        icon={LibraryIcon}
        actions={
          <button
            onClick={handleAddFolder}
            aria-label="Добавить папку с музыкой"
            title="Добавить папку с музыкой"
            className="flex shrink-0 items-center gap-2 rounded-xl bg-white/8 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-white/12"
          >
            <FolderOpen size={16} />
            {/* На телефоне подпись скрыта (иначе длинная кнопка распирала шапку
                по ширине) — остаётся понятная иконка с aria-label/title. */}
            <span className="hidden sm:inline">Добавить папку с музыкой</span>
          </button>
        }
      />

      {/* Вкладки — подчёркнутая полоса со счётчиками (как на макете). */}
      <TabBar tabs={tabs} active={tab} onChange={setTab} />

      {/* Треки */}
      {tab === "tracks" && (
        <div className="flex flex-col">
          {allTracks.length > 0 ? (
            allTracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={track}
                index={i}
                active={activeTrackId === track.id}
                playing={isPlaying}
                onPlay={(t) => playTrack(t, allTracks)}
                showAlbum
              />
            ))
          ) : (
            <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-20 text-center text-slate-400">
              <div>
                <FolderOpen size={40} className="mx-auto mb-3 opacity-40" />
                <p className="mt-2">Нет треков. Добавьте папку с музыкой или перейдите в «Локальную музыку».</p>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Альбомы */}
      {tab === "albums" && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-5">
          {Array.from(albumsMap.entries()).map(([key, tracks]) => {
            const album = tracks[0];
            const albumName = album.album!;
            const artist = album.artist;
            return (
              <button
                key={key}
                onClick={() => navigate(`/albums/${encodeURIComponent(key)}`)}
                className="group flex flex-col text-left"
              >
                <div className="relative w-full">
                  <CoverFromTrack
                    track={album}
                    alt={albumName}
                    className="aspect-square w-full"
                    iconSize={36}
                    neon
                  />
                  <TypeBadge label="Альбом" />
                </div>
                <strong className="mt-3 truncate text-[15px] font-semibold text-white group-hover:text-[#c084fc]">
                  {albumName}
                </strong>
                <span className="truncate text-sm text-slate-400"><ArtistLink name={artist} /> · {pluralWithCount(tracks.length, "трек", "трека", "треков")}</span>
              </button>
            );
          })}
          {albumsMap.size === 0 && (
            <div className="col-span-full grid place-items-center py-12 text-center text-slate-400">
              <p>Альбомы появятся после добавления треков.</p>
            </div>
          )}
        </div>
      )}

      {/* Артисты */}
      {tab === "artists" && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-5">
          {Array.from(artistsMap.entries()).map(([artist, tracks]) => {
            const sample = tracks[0];
            return (
              <button
                type="button"
                key={artist}
                onClick={() => navigate(`/artist/${encodeURIComponent(artist)}`)}
                aria-label={`Открыть страницу исполнителя «${artist}»`}
                className="group flex flex-col items-center text-center"
              >
                <div className="relative w-full">
                  <CoverFromTrack
                    track={sample}
                    alt={artist}
                    className="aspect-square w-full"
                    rounded="rounded-full"
                    iconSize={36}
                    neon
                  />
                  <TypeBadge label="Артист" />
                </div>
                <strong className="mt-3 truncate text-[15px] font-semibold text-white group-hover:text-[#c084fc]">{artist}</strong>
                <span className="text-sm text-slate-400">{pluralWithCount(tracks.length, "трек", "трека", "треков")}</span>
              </button>
            );
          })}
          {artistsMap.size === 0 && (
            <div className="col-span-full grid place-items-center py-12 text-center text-slate-400">
              <p>Артисты появятся после добавления треков.</p>
            </div>
          )}
        </div>
      )}

      {/* Плейлисты */}
      {tab === "playlists" && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-5">
          {playlists.map((p) => (
            <PlaylistCard key={p.id} playlist={p} onOpen={() => navigate(`/playlists/${p.id}`)} />
          ))}
          {playlists.length === 0 && (
            <div className="col-span-full grid place-items-center py-12 text-center text-slate-400">
              <p>Создайте первый плейлист.</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
