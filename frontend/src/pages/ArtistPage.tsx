import { ArrowLeft, Play, Music2, ExternalLink } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTrackListPlayer } from "../store/playerStore";
import { getArtist } from "../shared/api/client";
import { TrackRow } from "../shared/ui/TrackRow";
import { Cover } from "../shared/ui/Cover";
import { TabBar, type TabItem } from "../shared/ui/TabBar";
import { pluralWithCount } from "../shared/lib/format";
import type { ArtistRelease } from "../shared/types";

type MusicTab = "popular" | "albums" | "singles" | "appears";

/**
 * Страница исполнителя в духе Spotify: фото, популярные треки и релизы
 * (альбомы/синглы) с обложками. Данные собирает backend (App.GetArtist):
 * структурированный каталог (Yandex с токеном) + мультипоиск по рабочим
 * источникам, где в приоритете «нецензурные» версии с YouTube/SoundCloud.
 * Трек-лист релиза открывается на отдельной странице альбома по клику.
 */
export function ArtistPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const [tab, setTab] = useState<MusicTab>("popular");

  const artistName = name ? decodeURIComponent(name) : "";

  const { data: artist, isFetching, isError } = useQuery({
    queryKey: ["artist", artistName],
    queryFn: () => getArtist(artistName),
    enabled: artistName.length > 0,
    staleTime: 5 * 60 * 1000,
  });

  const topTracks = artist?.topTracks ?? [];
  const albums = artist?.albums ?? [];
  const singles = artist?.singles ?? [];
  // «Встречается в» — релизы других исполнителей, где артист лишь участвует
  // (совместки, сборники). Раньше они попадали в «Альбомы» и выглядели как
  // чужая дискография в его карточке — теперь у них отдельная вкладка.
  const appearsOn = artist?.appearsOn ?? [];
  const hasReleases = albums.length > 0 || singles.length > 0 || appearsOn.length > 0;

  // «Популярные релизы» — все релизы, самые свежие сверху.
  const popularReleases = useMemo(
    () => [...albums, ...singles].sort((a, b) => (b.year ?? 0) - (a.year ?? 0)),
    [albums, singles],
  );

  const coverSrc = artist?.coverUrl || topTracks.find((t) => t.coverUrl)?.coverUrl;

  const playAll = () => {
    if (topTracks.length > 0) playTrack(topTracks[0], topTracks);
  };

  // Клик по релизу открывает страницу альбома с полным трек-листом
  // (подгружается по service+id через App.GetAlbumTracks). Мы больше не
  // включаем трек сразу — сначала показываем содержимое альбома, как в Spotify.
  const openRelease = (release: ArtistRelease) => {
    navigate(`/albums/${encodeURIComponent(release.id || release.title)}`, {
      state: {
        album: {
          service: release.service,
          albumId: release.id,
          title: release.title,
          artist: artistName,
          coverUrl: release.coverUrl,
          externalUrl: release.externalUrl,
          kind: release.kind,
          year: release.year,
        },
      },
    });
  };

  const musicTabs: TabItem<MusicTab>[] = [];
  if (popularReleases.length) musicTabs.push({ id: "popular", label: "Популярные релизы", count: popularReleases.length });
  if (albums.length) musicTabs.push({ id: "albums", label: "Альбомы", count: albums.length });
  if (singles.length) musicTabs.push({ id: "singles", label: "Синглы и EP", count: singles.length });
  if (appearsOn.length) musicTabs.push({ id: "appears", label: "Встречается в", count: appearsOn.length });

  // Активная вкладка должна существовать: если выбранной категории нет (например,
  // у артиста только «встречается в»), берём первую доступную.
  const activeTab = musicTabs.some((t) => t.id === tab) ? tab : musicTabs[0]?.id ?? "popular";
  const visibleReleases =
    activeTab === "albums" ? albums : activeTab === "singles" ? singles : activeTab === "appears" ? appearsOn : popularReleases;

  return (
    <div className="flex flex-col gap-8 py-2">
      <button
        onClick={() => navigate(-1)}
        className="flex w-fit items-center gap-2 text-sm text-slate-400 transition-colors hover:text-white"
      >
        <ArrowLeft size={16} />
        Назад
      </button>

      {/* Шапка исполнителя */}
      <div className="flex flex-col items-center gap-4 text-center sm:flex-row sm:items-end sm:gap-6 sm:text-left">
        <Cover
          accent="violet"
          src={coverSrc}
          alt={artistName}
          className="h-36 w-36 shrink-0 shadow-2xl sm:h-44 sm:w-44"
          rounded="rounded-full"
          iconSize={60}
          neon
        />
        <div className="flex min-w-0 flex-col items-center sm:items-start">
          <p className="text-sm font-medium uppercase tracking-wide text-slate-400">Исполнитель</p>
          <h1 className="mt-1 max-w-full truncate text-3xl font-black text-white sm:text-5xl">{artistName}</h1>
          <p className="mt-2 text-sm text-slate-400">
            {isFetching
              ? "Собираем страницу исполнителя…"
              : `${pluralWithCount(topTracks.length, "популярный трек", "популярных трека", "популярных треков")}${
                  hasReleases ? ` · ${pluralWithCount(albums.length + singles.length, "релиз", "релиза", "релизов")}` : ""
                }`}
          </p>
          <button
            onClick={playAll}
            disabled={topTracks.length === 0}
            className="mt-5 flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-2.5 text-sm font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-[1.03] disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Play size={18} fill="currentColor" />
            Слушать
          </button>
        </div>
      </div>

      {isError && (
        <div className="rounded-2xl border border-white/8 bg-white/5 p-8 text-center text-slate-400">
          Не удалось загрузить страницу исполнителя. Попробуйте позже.
        </div>
      )}

      {!isError && !isFetching && topTracks.length === 0 && !hasReleases && (
        <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
          <div>
            <Music2 size={40} className="mx-auto mb-3 opacity-40" />
            <p>По этому исполнителю ничего не нашлось.</p>
          </div>
        </div>
      )}

      {/* Популярные треки */}
      {topTracks.length > 0 && (
        <section>
          <h2 className="mb-3 text-xl font-bold text-white">Популярные треки</h2>
          <div className="flex flex-col">
            {topTracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={track}
                index={i}
                active={activeTrackId === track.id}
                playing={isPlaying}
                onPlay={(t) => playTrack(t, topTracks)}
                showAlbum
              />
            ))}
          </div>
        </section>
      )}

      {/* Музыка: релизы по вкладкам */}
      {hasReleases && (
        <section className="flex flex-col gap-4">
          <h2 className="text-xl font-bold text-white">Музыка</h2>
          <TabBar tabs={musicTabs} active={activeTab} onChange={setTab} />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-5">
            {visibleReleases.map((release) => (
              <ReleaseCard
                key={`${release.service}:${release.id}:${release.title}`}
                release={release}
                onOpen={() => openRelease(release)}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

const KIND_LABEL: Record<ArtistRelease["kind"], string> = {
  album: "Альбом",
  single: "Сингл",
  ep: "EP",
};

function ReleaseCard({
  release,
  onOpen,
}: {
  release: ArtistRelease;
  onOpen: () => void;
}) {
  const meta = [release.year ? String(release.year) : null, KIND_LABEL[release.kind]]
    .filter(Boolean)
    .join(" · ");
  const openable = Boolean(release.id) || Boolean(release.externalUrl);
  return (
    <button
      type="button"
      onClick={onOpen}
      disabled={!openable}
      aria-label={`Открыть релиз «${release.title}»`}
      className="group flex flex-col text-left disabled:cursor-default"
    >
      <div className="relative w-full">
        <Cover
          accent="violet"
          src={release.coverUrl}
          alt={release.title}
          className="aspect-square w-full"
          iconSize={36}
          neon
        />
        {!release.id && release.externalUrl && (
          <span className="absolute right-2 top-2 grid h-7 w-7 place-items-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100">
            <ExternalLink size={14} />
          </span>
        )}
      </div>
      <strong className="mt-3 truncate text-[15px] font-semibold text-white group-hover:text-[#c084fc]">
        {release.title}
      </strong>
      <span className="truncate text-sm text-slate-400">{meta}</span>
    </button>
  );
}

export default ArtistPage;
