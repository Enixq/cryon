import { Search as SearchIcon, X, Play } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useUiStore } from "../store/uiStore";
import { useTrackListPlayer } from "../store/playerStore";
import { listPlaylists, search } from "../shared/api/client";
import { SOURCES } from "../shared/sources";
import { genres } from "../shared/data/genres";
import { TrackRow } from "../shared/ui/TrackRow";
import { PlaylistCard } from "../shared/ui/PlaylistCard";
import { Cover } from "../shared/ui/Cover";
import { GenreCard } from "../shared/ui/GenreCard";
import { PageHeader } from "../shared/ui/PageHeader";
import { TabBar, type TabItem } from "../shared/ui/TabBar";
import { cn } from "../shared/lib/cn";
import { pluralWithCount } from "../shared/lib/format";
import type { SourceId, Track } from "../shared/types";

type Filter = "all" | "tracks" | "albums" | "artists" | "playlists";

const searchableSources: SourceId[] = ["youtube", "soundcloud", "yandex", "local"];

// Порядок предпочтения источника для «основного» значка на карточке альбома,
// когда релиз найден сразу в нескольких сервисах (нецензурный SoundCloud выше).
const sourcePriority: SourceId[] = ["soundcloud", "yandex", "youtube", "spotify", "vk", "local"];

function primarySource(sources: Set<SourceId>): SourceId {
  for (const id of sourcePriority) {
    if (sources.has(id)) return id;
  }
  return [...sources][0] ?? "youtube";
}

// normalizeText приводит строку к виду для сравнения: нижний регистр, «ё»→«е»,
// пунктуация → пробел, схлопнутые пробелы. Так «Ёлка!» и «елка» совпадают.
function normalizeText(s: string): string {
  return s
    .toLowerCase()
    .replace(/ё/g, "е")
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

// scoreText оценивает, насколько строка соответствует нормализованному запросу
// needle: точное совпадение → 100, начинается с запроса → 70, содержит → 45,
// все слова запроса присутствуют → 30, часть слов → пропорционально. Это же
// ранжирование используется и для исполнителей, и для треков, чтобы «нужный/
// близкий результат» всплывал наверх.
function scoreText(hayRaw: string, needle: string): number {
  const hay = normalizeText(hayRaw);
  if (!hay || !needle) return 0;
  if (hay === needle) return 100;
  if (hay.startsWith(needle)) return 70;
  if (hay.includes(needle)) return 45;
  const words = needle.split(" ").filter(Boolean);
  if (words.length > 1) {
    const hit = words.filter((w) => hay.includes(w)).length;
    if (hit === words.length) return 30;
    if (hit > 0) return Math.round((hit / words.length) * 20);
  }
  return 0;
}

// scoreTrack — релевантность трека запросу: берём лучшее из совпадений по
// названию и исполнителю, с небольшой добавкой, когда совпадает и то, и другое.
function scoreTrack(title: string, artist: string, needle: string): number {
  const t = scoreText(title, needle);
  const a = scoreText(artist, needle);
  return Math.max(t, a) + 0.2 * Math.min(t, a);
}

// Витринные агрегаты выдачи. sources — набор сервисов, где встретилась сущность
// (для дедупа альбомов/артистов и фильтра «по источнику» во всех разделах).
type AlbumAgg = { id: string; title: string; artist: string; coverUrl?: string; accent: Track["accent"]; sources: Set<SourceId>; tracks: Track[] };
type ArtistAgg = { id: string; name: string; coverUrl?: string; accent: Track["accent"]; sources: Set<SourceId>; tracks: Track[] };

export function SearchPage() {
  const query = useUiStore((s) => s.searchQuery);
  const setSearchQuery = useUiStore((s) => s.setSearchQuery);
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const [filter, setFilter] = useState<Filter>("all");
  const [selectedSources, setSelectedSources] = useState<SourceId[]>([]);
  // Клиентский фильтр отображения по источнику (Все / YouTube / SoundCloud / …).
  // Отличается от верхнего «Искать в источниках», который меняет backend-запрос.
  const [resultSource, setResultSource] = useState<SourceId | "all">("all");
  const navigate = useNavigate();

  const q = query.trim().toLowerCase();
  const trimmed = query.trim();

  // Треки ищем в реальных источниках через backend (в вебе — мок-фолбэк).
  // Прогрессивная выдача: вместо одного запроса, ждущего ВСЕ источники, шлём по
  // запросу на каждый источник и показываем результаты по мере готовности —
  // быстрые (локальная библиотека, тёплый кэш) появляются сразу, медленные
  // дозаполняются, а отказ одного источника не мешает остальным. Ищем в выбранных
  // источниках (чипсы «Искать в источниках»), при пустом выборе — во всех.
  const activeSources = selectedSources.length > 0 ? selectedSources : searchableSources;
  const trackQueries = useQueries({
    queries: activeSources.map((source) => ({
      queryKey: ["search", trimmed, source],
      queryFn: () => search(source, trimmed),
      enabled: trimmed.length > 0,
    })),
  });
  // Стабильная подпись выдачи: строка меняется только когда какой-то источник
  // обновил данные. По ней и пересобираем плоский список — иначе flatMap давал
  // бы новый массив на каждый рендер и агрегаты ниже считались бы вхолостую.
  const tracksSignature = trackQueries.map((qr) => qr.dataUpdatedAt).join("|");
  const foundTracks = useMemo(
    () => trackQueries.flatMap((qr) => qr.data ?? []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tracksSignature],
  );
  // «Идёт поиск», пока грузится хоть один источник; «ошибка» — только если ВСЕ
  // источники упали (частичный сбой переживаем — показываем, что пришло).
  const isFetching = trackQueries.some((qr) => qr.isFetching);
  const isError = trackQueries.length > 0 && trackQueries.every((qr) => qr.isError);

  const { data: userPlaylists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
  });

  // Производные сущности строим только из реальной выдачи. Внешние сервисы
  // отдают треки, поэтому отдельного API каталога альбомов/артистов не нужно.
  // Артистов сортируем по числу треков — самый заметный идёт в «геро»-карточку.
  const results = useMemo(() => {
    if (!q) return null;
    const needle = normalizeText(q);
    // Сортируем треки по релевантности запросу ещё ДО агрегации, чтобы «нужный/
    // близкий» трек всплывал первым и попадал в затравку альбомов/исполнителей.
    // Сортировка стабильна: при равном скоре сохраняется исходный порядок выдачи.
    const tracks = [...(foundTracks ?? [])].sort(
      (a, b) => scoreTrack(b.title, b.artist, needle) - scoreTrack(a.title, a.artist, needle),
    );
    const albumsByKey = new Map<string, AlbumAgg>();
    const artistsByKey = new Map<string, ArtistAgg>();
    for (const track of tracks) {
      if (track.album) {
        // Дедуп альбома по «название|исполнитель» (без источника), но копим набор
        // источников, где он встретился, — для значка и фильтра по источнику.
        const key = `${track.album}|${track.artist}`;
        const album = albumsByKey.get(key) ?? { id: encodeURIComponent(key), title: track.album, artist: track.artist, coverUrl: track.coverUrl, accent: track.accent, sources: new Set<SourceId>(), tracks: [] };
        album.tracks.push(track);
        album.sources.add(track.source);
        if (!album.coverUrl && track.coverUrl) album.coverUrl = track.coverUrl;
        albumsByKey.set(key, album);
      }
      // Исполнитель трека — это склейка Artists[] через запятую. Разбираем её
      // обратно на отдельных артистов, иначе совместка «A, B, C» превращается
      // в один фиктивный «артист» с этим длинным именем (как было на скрине).
      if (track.artist && track.artist !== "Неизвестный исполнитель") {
        const seen = new Set<string>();
        for (const raw of track.artist.split(",")) {
          const name = raw.trim();
          if (!name) continue;
          const key = name.toLowerCase();
          if (seen.has(key)) continue; // один трек не должен считаться артисту дважды
          seen.add(key);
          const artist = artistsByKey.get(key) ?? {
            id: name.toLowerCase(),
            name,
            coverUrl: track.coverUrl,
            accent: track.accent,
            sources: new Set<SourceId>(),
            tracks: [],
          };
          artist.tracks.push(track);
          artist.sources.add(track.source);
          if (!artist.coverUrl && track.coverUrl) artist.coverUrl = track.coverUrl;
          artistsByKey.set(key, artist);
        }
      }
    }
    // Ранжируем исполнителей тем же scoreText, что и треки: «ведущим» становится
    // именно искомый артист, а не тот, у кого случайно больше треков в выдаче.
    const scoreArtist = (name: string) => scoreText(name, needle);
    let artists = [...artistsByKey.values()].sort((a, b) => {
      const s = scoreArtist(b.name) - scoreArtist(a.name);
      if (s !== 0) return s;
      return b.tracks.length - a.tracks.length;
    });
    // Если запрос совпадает с именами исполнителей — показываем только их, а не
    // всех гостей-соисполнителей из выдачи (запрос артиста → его карточка(и),
    // а не десяток случайных). Для запроса-названия трека оставляем всех.
    const relevant = artists.filter((a) => scoreArtist(a.name) > 0);
    if (relevant.length > 0) artists = relevant;
    artists = artists.slice(0, 8);
    return {
      tracks,
      albums: [...albumsByKey.values()],
      artists,
      playlists: userPlaylists.filter((p) => p.title.toLowerCase().includes(q) || p.description.toLowerCase().includes(q)),
    };
  }, [q, foundTracks, userPlaylists]);

  // Источники, реально присутствующие в выдаче (для ряда чипсов-фильтров), с
  // числом треков у каждого. Порядок — по убыванию количества.
  const sourceCounts = useMemo(() => {
    const counts = new Map<SourceId, number>();
    for (const t of results?.tracks ?? []) counts.set(t.source, (counts.get(t.source) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  }, [results]);

  // Сбрасываем фильтр источника, если выбранного источника нет в новой выдаче.
  useEffect(() => {
    if (resultSource !== "all" && !sourceCounts.some(([id]) => id === resultSource)) {
      setResultSource("all");
    }
  }, [sourceCounts, resultSource]);

  // Применяем фильтр источника ко всем разделам (треки/исполнители/альбомы).
  // Плейлисты — пользовательские, под источники сервисов не попадают.
  const view = useMemo(() => {
    if (!results) return null;
    if (resultSource === "all") return results;
    return {
      tracks: results.tracks.filter((t) => t.source === resultSource),
      albums: results.albums.filter((a) => a.sources.has(resultSource)),
      artists: results.artists.filter((a) => a.sources.has(resultSource)),
      playlists: results.playlists,
    };
  }, [results, resultSource]);

  const totalFound = view
    ? view.tracks.length + view.albums.length + view.artists.length + view.playlists.length
    : 0;

  // Вкладки-фильтры с подчёркиванием и счётчиками — как на макете поиска.
  // Счётчики отражают текущий фильтр по источнику (view), а не всю выдачу.
  const filterTabs: TabItem<Filter>[] = view
    ? [
        { id: "all", label: "Всё" },
        { id: "tracks", label: "Треки", count: view.tracks.length },
        { id: "artists", label: "Исполнители", count: view.artists.length },
        { id: "albums", label: "Альбомы", count: view.albums.length },
        { id: "playlists", label: "Плейлисты", count: view.playlists.length },
      ]
    : [];

  const showTracks = view && (filter === "all" || filter === "tracks") && view.tracks.length > 0;
  const showAlbums = view && (filter === "all" || filter === "albums") && view.albums.length > 0;
  const showPlaylists = view && (filter === "all" || filter === "playlists") && view.playlists.length > 0;

  // «Геро»-карточка ведущего исполнителя: показывается на вкладках «Всё» и
  // «Исполнители», когда в выдаче есть артисты. Занимает всю ширину над
  // двухколоночным блоком результатов.
  const featured = view && view.artists.length > 0 ? view.artists[0] : null;
  const showFeatured = featured && (filter === "all" || filter === "artists");
  // Ведущего уже показывает геро-карточка — в сетке выводим ОСТАЛЬНЫХ, чтобы он
  // не дублировался. Если артист всего один, сетка пустая и секция скрыта.
  const gridArtists = (showFeatured ? view?.artists.slice(1) : view?.artists) ?? [];
  const showArtists = view && (filter === "all" || filter === "artists") && gridArtists.length > 0;

  return (
    <div className="flex flex-col gap-6 py-2">
      <div>
        <PageHeader title="Поиск" icon={SearchIcon} />

        {/* Рабочее поле ввода: привязано к тому же searchQuery, что и строка
            сверху (App.SearchAll). Раньше здесь была статичная подсказка. */}
        <div className="mt-4 flex h-12 max-w-2xl items-center gap-3 rounded-xl bg-white/5 px-4 focus-within:bg-white/8">
          <SearchIcon size={18} className="text-slate-400" />
          <input
            type="text"
            autoFocus
            value={query}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Начните вводить название трека, исполнителя, альбома или плейлиста"
            className="flex-1 bg-transparent text-sm text-white outline-none placeholder:text-slate-500"
            aria-label="Поиск по трекам, исполнителям, альбомам и плейлистам"
          />
          {query && (
            <button
              onClick={() => setSearchQuery("")}
              className="grid h-6 w-6 place-items-center rounded-full text-slate-400 transition-colors hover:bg-white/10 hover:text-white"
              aria-label="Очистить поиск"
            >
              <X size={16} />
            </button>
          )}
        </div>

        <div className="mt-4" aria-label="Источники поиска">
          <p className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">Искать в источниках</p>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setSelectedSources([])}
              className={cn("rounded-full px-3 py-1.5 text-sm transition-colors", selectedSources.length === 0 ? "bg-white text-black" : "bg-white/8 text-slate-300 hover:bg-white/12")}
              aria-pressed={selectedSources.length === 0}
            >Все</button>
            {searchableSources.map((source) => {
              const selected = selectedSources.includes(source);
              return <button
                type="button"
                key={source}
                onClick={() => setSelectedSources((current) => selected ? current.filter((id) => id !== source) : [...current, source])}
                className={cn("rounded-full px-3 py-1.5 text-sm transition-colors", selected ? "bg-[#a78bfa] text-black" : "bg-white/8 text-slate-300 hover:bg-white/12")}
                aria-pressed={selected}
              >{SOURCES[source].name}</button>;
            })}
          </div>
        </div>

        {trimmed && (
          <p className="mt-4 text-sm text-slate-400">
            {isFetching
              ? "Идёт поиск в выбранных источниках…"
              : isError
                ? "Не удалось получить результаты из источников"
                : `Найдено ${pluralWithCount(totalFound, "результат", "результата", "результатов")}`}
          </p>
        )}
      </div>

      {!results ? (
        <section>
          <h2 className="mb-4 text-xl font-bold text-white">Обзор жанров</h2>
          <p className="mb-4 -mt-2 text-sm text-slate-400">
            Нажмите на жанр, чтобы открыть подборку треков.
          </p>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
            {genres.map((g) => (
              <GenreCard key={g.slug} genre={g} onClick={() => navigate(`/genre/${g.slug}`)} />
            ))}
          </div>
        </section>
      ) : (
        <>
          {/* Фильтры — подчёркнутые вкладки со счётчиками (как на макете). */}
          <TabBar tabs={filterTabs} active={filter} onChange={setFilter} />

          {/* Градация по источнику: фильтрует все разделы (треки/исполнители/
              альбомы). Показываем только когда в выдаче больше одного источника. */}
          {sourceCounts.length > 1 && (
            <div className="-mt-2 flex flex-wrap gap-2" role="group" aria-label="Фильтр по источнику">
              <button
                type="button"
                onClick={() => setResultSource("all")}
                className={cn("rounded-full px-3 py-1.5 text-sm transition-colors", resultSource === "all" ? "bg-white text-black" : "bg-white/8 text-slate-300 hover:bg-white/12")}
                aria-pressed={resultSource === "all"}
              >Все источники</button>
              {sourceCounts.map(([id, count]) => (
                <button
                  type="button"
                  key={id}
                  onClick={() => setResultSource(id)}
                  className={cn("flex items-center gap-2 rounded-full px-3 py-1.5 text-sm transition-colors", resultSource === id ? "bg-white text-black" : "bg-white/8 text-slate-300 hover:bg-white/12")}
                  aria-pressed={resultSource === id}
                >
                  <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: SOURCES[id].dot }} />
                  {SOURCES[id].name}
                  <span className={cn("text-xs", resultSource === id ? "text-black/60" : "text-slate-500")}>{count}</span>
                </button>
              ))}
            </div>
          )}

          {totalFound === 0 && !isFetching && (
            <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
              <div>
                <SearchIcon size={40} className="mx-auto mb-3 opacity-40" />
                <p>Ничего не найдено. Попробуйте изменить запрос.</p>
              </div>
            </div>
          )}

          {/* Ведущий исполнитель — крупная карточка над двумя колонками. */}
          {showFeatured && featured && (
            <FeaturedArtist
              artist={featured}
              onOpen={() => navigate(`/artist/${encodeURIComponent(featured.name)}`)}
              onPlay={() => featured.tracks[0] && playTrack(featured.tracks[0], featured.tracks)}
            />
          )}

          {/* Двухколоночная раскладка: слева — треки, справа — исполнители,
              альбомы и плейлисты. На узких экранах колонки складываются. */}
          <div className="grid gap-8 lg:grid-cols-[1.6fr_1fr]">
            <div className="flex flex-col gap-8">
              {showTracks && (
                <section>
                  <h2 className="mb-3 text-xl font-bold text-white">Треки</h2>
                  <div className="flex flex-col">
                    {view?.tracks.map((track, i) => (
                      <TrackRow
                        key={track.id}
                        track={track}
                        index={i}
                        active={activeTrackId === track.id}
                        playing={isPlaying}
                        onPlay={(t) => playTrack(t, view?.tracks ?? [])}
                        showAlbum
                      />
                    ))}
                  </div>
                </section>
              )}
            </div>

            <div className="flex flex-col gap-8">
              {showArtists && (
                <section>
                  <h2 className="mb-3 text-xl font-bold text-white">Исполнители</h2>
                  <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 sm:gap-5 lg:grid-cols-2">
                    {gridArtists.map((a) => (
                      <button
                        type="button"
                        key={a.id}
                        onClick={() => navigate(`/artist/${encodeURIComponent(a.name)}`)}
                        aria-label={`Открыть страницу исполнителя «${a.name}»`}
                        className="group flex flex-col items-center text-center"
                      >
                        <Cover accent={a.accent} src={a.coverUrl} alt={a.name} className="aspect-square w-full" rounded="rounded-full" iconSize={36} neon />
                        <strong className="mt-3 truncate text-[15px] font-semibold text-white group-hover:text-[#c084fc]">{a.name}</strong>
                        <span className="text-sm text-slate-400">{pluralWithCount(a.tracks.length, "трек", "трека", "треков")}</span>
                      </button>
                    ))}
                  </div>
                </section>
              )}

              {showAlbums && (
                <section>
                  <h2 className="mb-3 text-xl font-bold text-white">Альбомы</h2>
                  <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 sm:gap-5 lg:grid-cols-2">
                    {view?.albums.map((a) => (
                      <button
                        key={a.id}
                        onClick={() =>
                          navigate(`/albums/${a.id}`, {
                            state: {
                              album: {
                                title: a.title,
                                artist: a.artist,
                                coverUrl: a.coverUrl,
                                accent: a.accent,
                                seedTracks: a.tracks,
                              },
                            },
                          })
                        }
                        className="group flex flex-col text-left"
                      >
                        <Cover accent={a.accent} src={a.coverUrl} alt={a.title} className="aspect-square w-full" iconSize={36} neon />
                        <strong className="mt-3 truncate text-[15px] font-semibold text-white group-hover:text-[#c084fc]">{a.title}</strong>
                        <span className="flex items-center gap-1.5 truncate text-sm text-slate-400">
                          <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: SOURCES[primarySource(a.sources)].dot }} />
                          <span className="truncate">{a.artist} · {SOURCES[primarySource(a.sources)].name}</span>
                        </span>
                      </button>
                    ))}
                  </div>
                </section>
              )}

              {showPlaylists && (
                <section>
                  <h2 className="mb-3 text-xl font-bold text-white">Плейлисты</h2>
                  <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 sm:gap-5 lg:grid-cols-2">
                    {view?.playlists.map((p) => (
                      <PlaylistCard key={p.id} playlist={p} onOpen={() => navigate(`/playlists/${p.id}`)} />
                    ))}
                  </div>
                </section>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

// FeaturedArtist — крупная карточка ведущего исполнителя в выдаче: большая
// круглая обложка, имя, число треков и кнопка воспроизведения. Клик по карточке
// открывает страницу исполнителя (как в Spotify), кнопка ▶ играет его треки.
function FeaturedArtist({
  artist,
  onOpen,
  onPlay,
}: {
  artist: { name: string; coverUrl?: string; accent: Track["accent"]; tracks: Track[] };
  onOpen: () => void;
  onPlay: () => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
      aria-label={`Открыть страницу исполнителя «${artist.name}»`}
      className="group relative flex cursor-pointer items-center gap-4 overflow-hidden rounded-2xl border border-white/8 bg-white/5 p-4 transition-colors hover:bg-white/8 sm:gap-5 sm:p-5"
    >
      <Cover accent={artist.accent} src={artist.coverUrl} alt={artist.name} className="h-24 w-24 shrink-0 sm:h-28 sm:w-28" rounded="rounded-full" iconSize={44} neon />
      <div className="min-w-0 flex-1">
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--app-accent)]">Ведущий исполнитель</span>
        <h3 className="mt-1 truncate text-xl font-bold text-white group-hover:text-[#c084fc] sm:text-2xl">{artist.name}</h3>
        <p className="mt-0.5 text-sm text-slate-400">{pluralWithCount(artist.tracks.length, "трек", "трека", "треков")} в выдаче</p>
      </div>
      <button
        onClick={(e) => {
          e.stopPropagation();
          onPlay();
        }}
        className="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[#a855f7] text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105 sm:h-14 sm:w-14"
        aria-label={`Воспроизвести треки исполнителя «${artist.name}»`}
      >
        <Play size={22} fill="currentColor" className="ml-0.5" />
      </button>
    </div>
  );
}
