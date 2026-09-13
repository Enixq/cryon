import { Music2, Play } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { usePlayerStore } from "../store/playerStore";
import { SectionHeader } from "../shared/ui/SectionHeader";
import { PageHeader } from "../shared/ui/PageHeader";
import { TrackCard } from "../shared/ui/TrackCard";
import { RecoTrackCard } from "../shared/ui/RecoTrackCard";
import { SmartPlaylistCard } from "../shared/ui/SmartPlaylistCard";
import { Cover } from "../shared/ui/Cover";
import { useTrackCover } from "../shared/lib/useTrackCover";
import {
  listDailyMix,
  listFavorites,
  listHistory,
  listLocalTracks,
  listNewReleases,
  listPlaylists,
  listRecommendations,
  listWeeklyDiscoveries,
  onFavoritesChanged,
  onLastFMChanged,
  onRecoChanged,
  type UserPlaylistDto,
} from "../shared/api/client";
import { quickMixes } from "../mocks/data";
import type { QuickMix, Track } from "../shared/types";
import { pluralWithCount } from "../shared/lib/format";
import { deriveRecommendations, deriveSmartPlaylists } from "../shared/lib/recommendations";
import { useRecoFeedback } from "../shared/lib/useRecoFeedback";

// Приветствие по времени суток.
function greeting(): string {
  const h = new Date().getHours();
  if (h < 6) return "Доброй ночи";
  if (h < 12) return "Доброе утро";
  if (h < 18) return "Добрый день";
  return "Добрый вечер";
}

export function HomePage() {
  const playTrack = usePlayerStore((s) => s.playTrack);
  const navigate = useNavigate();

  const { data: history = [] } = useQuery({
    queryKey: ["history"],
    queryFn: listHistory,
  });

  const { data: playlists = [] } = useQuery({
    queryKey: ["playlists"],
    queryFn: listPlaylists,
  });

  const { data: favorites = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
  });

  const { data: localTracks = [] } = useQuery({
    queryKey: ["localTracks"],
    queryFn: listLocalTracks,
  });

  // Рекомендации из backend (Last.fm / оффлайн-фолбэк). Пусто в вебе без Wails
  // или когда вкусы ещё не набраны — тогда ниже показываем клиентские подборки.
  const { data: backendRecommendations = [], isLoading: recsLoading } = useQuery({
    queryKey: ["recommendations"],
    queryFn: () => listRecommendations(12),
  });

  // Оценки рекомендаций. Дизлайкнутый трек скрываем сразу, не дожидаясь
  // пересборки подборки на backend — иначе нажатие выглядит как «ничего не
  // произошло».
  const feedback = useRecoFeedback();

  const queryClient = useQueryClient();
  // Обновляем подборки сразу после смены API-ключа Last.fm
  useEffect(() => {
    const offLastFM = onLastFMChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["recommendations"] });
      queryClient.invalidateQueries({ queryKey: ["autoMix", "daily"] });
      queryClient.invalidateQueries({ queryKey: ["autoMix", "weekly"] });
      queryClient.invalidateQueries({ queryKey: ["newReleases"] });
    });
    const offFavorites = onFavoritesChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
    });
    // Профиль вкусов изменился (оценка рекомендации). Помечаем подборки
    // устаревшими без немедленного перезапроса: пересборка на backend — это
    // десятки поисков, а пользователь обычно оценивает несколько карточек
    // подряд. Новая версия оценок входит в ключ кэша, поэтому следующий заход
    // на экран соберёт подборку заново. «Радар новинок» тоже зависит от
    // профиля: он упорядочивается по знакомым исполнителям.
    const offReco = onRecoChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["recommendations"], refetchType: "none" });
      queryClient.invalidateQueries({ queryKey: ["autoMix"], refetchType: "none" });
      queryClient.invalidateQueries({ queryKey: ["newReleases"], refetchType: "none" });
    });
    return () => {
      offLastFM();
      offFavorites();
      offReco();
    };
  }, [queryClient]);

  // Авто-подборки: «Микс дня» (стабилен сутки) и «Открытия недели» (стабильны
  // неделю). Пусто в вебе без Wails или пока вкусы не набраны.
  const { data: dailyMix = [], isLoading: dailyLoading } = useQuery({
    queryKey: ["autoMix", "daily"],
    queryFn: () => listDailyMix(18),
  });
  const { data: weeklyDiscoveries = [], isLoading: weeklyLoading } = useQuery({
    queryKey: ["autoMix", "weekly"],
    queryFn: () => listWeeklyDiscoveries(18),
  });
  const { data: newReleases = [] } = useQuery({
    queryKey: ["newReleases"],
    queryFn: () => listNewReleases(18),
  });

  // Недавно прослушанное: уникальные треки из истории (свежие сверху).
  const recent = useMemo(() => {
    const out: Track[] = [];
    const seen = new Set<string>();
    for (const entry of history) {
      if (seen.has(entry.track.id)) continue;
      seen.add(entry.track.id);
      out.push(entry.track);
      if (out.length >= 10) break;
    }
    return out;
  }, [history]);

  const handlePlayTrack = (track: Track) => playTrack(track, recent);
  const smartPlaylists = useMemo(
    () => deriveSmartPlaylists(favorites, history),
    [favorites, history],
  );
  // Клиентские подборки — фолбэк на случай, когда backend авто-подборок не дал.
  const derived = useMemo(
    () => deriveRecommendations(favorites, history, localTracks),
    [favorites, history, localTracks],
  );

  // Дизлайкнутые треки убираем из всех подборок сразу. scoreFor стабилен между
  // рендерами (мемоизирован по карте оценок), поэтому пересчёт идёт только при
  // смене данных или новой оценке.
  const rateScore = feedback.scoreFor;
  const recommended = useMemo(
    () => backendRecommendations.filter((t) => rateScore(t) >= 0),
    [backendRecommendations, rateScore],
  );
  const visibleDailyMix = useMemo(
    () => dailyMix.filter((t) => rateScore(t) >= 0),
    [dailyMix, rateScore],
  );
  const visibleWeekly = useMemo(
    () => weeklyDiscoveries.filter((t) => rateScore(t) >= 0),
    [weeklyDiscoveries, rateScore],
  );

  const quickMixTracks: Record<QuickMix["id"], Track[]> = {
    m1: visibleDailyMix,
    m2: newReleases,
    m3: favorites,
    m4: visibleWeekly,
  };
  // Клиентские подборки называются ровно так же, как авто-подборки backend
  // («Микс дня», «Открытия недели»), поэтому показываем их только когда
  // backend ничего не вернул (нет ключа Last.fm или вкусы ещё не набраны).
  // Иначе на странице оказывались две секции с одинаковыми названиями и
  // разным содержимым.
  const showDerived = visibleDailyMix.length === 0 && visibleWeekly.length === 0;

  return (
    <div className="flex min-w-0 flex-col gap-6 py-1 sm:gap-8 sm:py-2">
      {/* Приветствие */}
      <PageHeader title={greeting()} subtitle="Музыка для тебя, собранная со всех твоих сервисов" />

      {/* Быстрые миксы — витрина */}
      <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
        {quickMixes.map((mix) => {
          const mixTracks = quickMixTracks[mix.id];
          return (
            <QuickMixCard
              key={mix.id}
              mix={mix}
              coverTrack={mixTracks[0]}
              onPlay={() => {
                if (mixTracks.length > 0) playTrack(mixTracks[0], mixTracks);
              }}
              onOpen={() => navigate(`/collection/${mix.id}`)}
            />
          );
        })}
      </div>

      {/* Недавно прослушано */}
      <section>
        <SectionHeader title="Недавно прослушано" onAction={() => navigate("/history")} />
        {recent.length > 0 ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 sm:gap-5 xl:grid-cols-5">
            {recent.slice(0, 5).map((track) => (
              <TrackCard key={track.id} track={track} onPlay={handlePlayTrack} />
            ))}
          </div>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-12 text-center text-slate-400">
            <p>Начните слушать музыку — недавние треки появятся здесь.</p>
          </div>
        )}
      </section>

      {/* Специально для вас — рекомендации из backend (похожие артисты) */}
      {recommended.length > 0 ? (
        <section>
          <SectionHeader
            title="Специально для вас"
            subtitle="Похожее на то, что вы слушаете. Оцените — подборка станет точнее"
            onAction={() => navigate("/search")}
          />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-6">
            {recommended.slice(0, 6).map((track) => (
              <RecoTrackCard
                key={track.id}
                track={track}
                onPlay={(t) => playTrack(t, recommended)}
                score={rateScore(track)}
                onRate={feedback.rate}
                disabled={feedback.saving}
              />
            ))}
          </div>
        </section>
      ) : recsLoading ? (
        <section>
          <SectionHeader
            title="Специально для вас"
            subtitle="Подбираем похожее на то, что вы слушаете…"
          />
          <RecoCardSkeleton />
        </section>
      ) : null}

      {/* Микс дня — авто-подборка, обновляется раз в сутки */}
      {visibleDailyMix.length > 0 ? (
        <section>
          <SectionHeader title="Микс дня" subtitle="Обновляется каждый день" />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-6">
            {visibleDailyMix.slice(0, 6).map((track) => (
              <RecoTrackCard
                key={track.id}
                track={track}
                onPlay={(t) => playTrack(t, visibleDailyMix)}
                score={rateScore(track)}
                onRate={feedback.rate}
                disabled={feedback.saving}
              />
            ))}
          </div>
        </section>
      ) : dailyLoading ? (
        <section>
          <SectionHeader title="Микс дня" subtitle="Собираем микс на сегодня…" />
          <RecoCardSkeleton />
        </section>
      ) : null}

      {/* Открытия недели — уклон в новых артистов, обновляется раз в неделю */}
      {visibleWeekly.length > 0 ? (
        <section>
          <SectionHeader title="Открытия недели" subtitle="Обновляется каждую неделю" />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-6">
            {visibleWeekly.slice(0, 6).map((track) => (
              <RecoTrackCard
                key={track.id}
                track={track}
                onPlay={(t) => playTrack(t, visibleWeekly)}
                score={rateScore(track)}
                onRate={feedback.rate}
                disabled={feedback.saving}
              />
            ))}
          </div>
        </section>
      ) : weeklyLoading ? (
        <section>
          <SectionHeader title="Открытия недели" subtitle="Ищем новых артистов для вас…" />
          <RecoCardSkeleton />
        </section>
      ) : null}

      {/* Умные плейлисты */}
      <section>
        <SectionHeader title="Умные плейлисты" onAction={() => navigate("/playlists")} />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4">
          {smartPlaylists.map((playlist) => (
            <SmartPlaylistCard
              key={playlist.id}
              playlist={playlist}
              onOpen={() => navigate(`/smart/${playlist.id}`)}
              onPlay={() => playlist.tracks[0] && playTrack(playlist.tracks[0], playlist.tracks)}
            />
          ))}
        </div>
      </section>

      {/* Рекомендации — клиентский фолбэк, когда backend авто-подборок не дал */}
      {showDerived && (
        <section>
          <SectionHeader title="Рекомендации" onAction={() => navigate("/playlists")} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {derived.map((playlist) => (
              <SmartPlaylistCard
                key={playlist.id}
                playlist={playlist}
                onOpen={() => navigate(`/smart/${playlist.id}`)}
                onPlay={() => playlist.tracks[0] && playTrack(playlist.tracks[0], playlist.tracks)}
              />
            ))}
          </div>
        </section>
      )}

      {/* Ваши плейлисты */}
      <section>
        <SectionHeader title="Ваши плейлисты" onAction={undefined} />
        {playlists.length > 0 ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-5">
            {playlists.slice(0, 5).map((p) => (
              <HomePlaylistCard
                key={p.id}
                playlist={p}
                onOpen={() => navigate(`/playlists/${p.id}`)}
              />
            ))}
          </div>
        ) : (
          <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 py-16 text-center text-slate-400">
            <div>
              <Music2 size={40} className="mx-auto mb-3 opacity-40" />
              <p>У вас пока нет плейлистов. Создайте первый.</p>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

// Скелетон сетки рекомендаций: backend собирает подборку через десятки запросов
// к Last.fm/источникам — это заметные секунды. Раньше секции просто отсутствовали
// до готовности и «выскакивали», из-за чего казалось, что рекомендаций нет.
// Плитки повторяют раскладку RecoTrackCard (квадратная обложка + две строки).
function RecoCardSkeleton() {
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 sm:gap-5 xl:grid-cols-6">
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="flex animate-pulse flex-col">
          <div className="aspect-square w-full rounded-2xl bg-white/10" />
          <div className="mt-3 h-3.5 w-3/4 rounded bg-white/10" />
          <div className="mt-2 h-3 w-1/2 rounded bg-white/[0.07]" />
        </div>
      ))}
    </div>
  );
}

function HomePlaylistCard({ playlist, onOpen }: { playlist: UserPlaylistDto; onOpen: () => void }) {
  return (
    <button onClick={onOpen} className="group flex flex-col text-left">
      <div
        className="neon-frame flex aspect-square w-full items-center justify-center overflow-hidden rounded-2xl"
        style={{ background: "linear-gradient(135deg, #a855f7, #ec4899)" }}
      >
        <Music2 size={44} className="text-white/80" />
      </div>
      <strong className="mt-3 truncate text-[15px] font-semibold text-white">
        {playlist.title}
      </strong>
      <span className="mt-0.5 truncate text-xs text-slate-500">
        {pluralWithCount(playlist.trackCount, "трек", "трека", "треков")}
      </span>
    </button>
  );
}

function QuickMixCard({
  mix,
  coverTrack,
  onPlay,
  onOpen,
}: {
  mix: QuickMix;
  coverTrack?: Track;
  onPlay: () => void;
  onOpen: () => void;
}) {
  // Карточка — не <button>: внутри есть вторая кнопка («Воспроизвести»), а
  // вложенные <button> — невалидная разметка, на которую React ругается
  // предупреждением validateDOMNesting. Открытие и воспроизведение живут
  // рядом как два отдельных элемента управления. Обложка берётся из первого
  // трека подборки (как в Spotify), с градиентной заглушкой, пока пусто.
  const coverUrl = useTrackCover(coverTrack);
  return (
    <div className="neon-card group relative flex items-center overflow-hidden rounded-2xl">
      <button
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-2 p-2 sm:gap-4 sm:p-3 text-left"
      >
        <Cover
          accent={mix.accent}
          src={coverUrl}
          alt={mix.title}
          className="h-12 w-12 shrink-0 sm:h-16 sm:w-16"
          rounded="rounded-xl"
          iconSize={24}
        />
        <div className="min-w-0 flex-1">
          <div className="truncate font-semibold text-white">{mix.title}</div>
          <div className="truncate text-sm text-slate-400">{mix.subtitle}</div>
        </div>
      </button>
      <button
        onClick={onPlay}
        className="mr-4 grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[#a855f7] text-white opacity-0 shadow-lg shadow-[#a855f7]/40 transition-opacity group-hover:opacity-100"
        aria-label={`Воспроизвести: ${mix.title}`}
      >
        <Play size={16} fill="currentColor" className="ml-0.5" />
      </button>
    </div>
  );
}
