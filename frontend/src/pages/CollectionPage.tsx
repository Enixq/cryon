import { useEffect, useMemo } from "react";
import { Play, RefreshCw, Sparkles } from "lucide-react";
import { useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { TrackRow } from "../shared/ui/TrackRow";
import { PageHeader } from "../shared/ui/PageHeader";
import { useTrackListPlayer } from "../store/playerStore";
import { pluralWithCount } from "../shared/lib/format";
import { useRecoFeedback } from "../shared/lib/useRecoFeedback";
import {
  isWailsRuntime,
  listDailyMix,
  listNewReleases,
  listFavorites,
  listWeeklyDiscoveries,
  onFavoritesChanged,
  onLastFMChanged,
  onRecoChanged,
} from "../shared/api/client";
import { tracks as mockTracks } from "../mocks/data";
import type { Track } from "../shared/types";

/** Данные для веб-режима (без Wails-биндингов). */
function mockForKind(kind: string) {
  switch (kind) {
    case "m1":
      return mockTracks.filter((t) => t.liked || t.source === "youtube").slice(0, 20);
    case "m2":
    case "new-releases":
      return [...mockTracks].reverse().slice(0, 20);
    case "m3":
      return mockTracks.filter((t) => t.liked);
    case "m4":
      return mockTracks.filter((t) => t.source !== "local").slice(0, 20);
    default:
      return [];
  }
}

interface CollectionMeta {
  title: string;
  subtitle: string;
  /** Почему подборка пуста и что сделать — вместо безликого «Пусто». */
  emptyHint: string;
  /**
   * Подборку собрал движок рекомендаций — значит, её карточки можно оценивать,
   * и оценка влияет на следующую сборку. Избранное и «Радар новинок» оценивать
   * бессмысленно: это не рекомендации, а фактические списки.
   */
  rateable?: boolean;
}

// Названия совпадают с карточками быстрых миксов на главной, чтобы переход
// с главной не менял заголовок под пользователем.
const newReleasesMeta: CollectionMeta = {
  title: "Радар новинок",
  subtitle: "Свежие релизы из подключённых сервисов — сначала ваши исполнители",
  emptyHint:
    "Новинки приходят из подключённых сервисов. Подключите Yandex Music или Spotify в настройках — и релизы появятся здесь.",
};

const collectionMeta: Record<string, CollectionMeta> = {
  m1: {
    title: "Мой микс",
    subtitle: "На основе ваших прослушиваний, обновляется раз в сутки",
    emptyHint:
      "Микс собирается по вашим прослушиваниям и избранному. Послушайте несколько треков — и он появится здесь.",
    rateable: true,
  },
  m2: newReleasesMeta,
  "new-releases": newReleasesMeta,
  m3: {
    title: "Любимые треки",
    subtitle: "Всё, что вы отметили сердцем",
    emptyHint: "Пока ничего не отмечено. Нажмите сердце у трека — и он появится здесь.",
  },
  m4: {
    title: "Снято с повторов",
    subtitle: "Открытия недели: артисты, которых вы ещё не слушали",
    emptyHint:
      "Подборка обновляется раз в неделю по похожим артистам. Нужен ключ Last.fm в настройках и немного истории прослушиваний.",
    rateable: true,
  },
};

// Коллекция целиком приходит с backend. Моки — только наполнение для
// веб-разработки (vite dev без Wails), в приложении они не показываются.
export function CollectionPage() {
  const { kind } = useParams<{ kind: string }>();
  const { playTrack, isPlaying, activeTrackId } = useTrackListPlayer();
  const queryClient = useQueryClient();

  // При смене Last.fm-ключа перезагрузим все коллекции.
  useEffect(() => {
    const off = onLastFMChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["collection"] });
    });
    return off;
  }, [queryClient]);

  // Перезагружаем текущую коллекцию, когда изменилось избранное.
  useEffect(() => {
    const off = onFavoritesChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["collection", kind] });
    });
    return off;
  }, [queryClient, kind]);

  // Оценка меняет профиль вкусов, а от него зависят все подборки движка (в том
  // числе порядок «Радара новинок»). Помечаем устаревшими без перезапроса:
  // пересборка — это десятки поисков по источникам, а оценивают обычно
  // несколько карточек подряд. Свежая подборка придёт при следующем заходе.
  useEffect(() => {
    const off = onRecoChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["collection"], refetchType: "none" });
    });
    return off;
  }, [queryClient]);

  const queryKey = useMemo(() => ["collection", kind], [kind]);

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey,
    queryFn: async () => {
      if (!kind) return [];
      switch (kind) {
        case "m1":
          return listDailyMix(40);
        case "m2":
        case "new-releases":
          return listNewReleases(40);
        case "m3":
          return listFavorites();
        case "m4":
          return listWeeklyDiscoveries(40);
        default:
          return [];
      }
    },
    // Моки-заглушка только для веб-режима. В Wails её быть не должно: с
    // placeholderData запрос сразу считается успешным (isLoading никогда не
    // становится true), и вместо скелетона пользователь видел 20 демо-треков
    // с чужими обложками, по которым Play молчит.
    placeholderData: isWailsRuntime() ? undefined : () => (kind ? mockForKind(kind) : []),
    // Кэш 5 минут — не дёргаем бэкенд при каждом заходе.
    staleTime: 5 * 60 * 1000,
    // Не фэйлим страницу при ошибке — показываем ошибку с кнопкой «Обновить».
    retry: false,
  });

  const meta = (kind && collectionMeta[kind]) || {
    title: "Коллекция",
    subtitle: "",
    emptyHint: "Выберите коллекцию из навигации.",
  };

  // Оценки нужны только подборкам движка. Хук вызывается всегда (правила
  // хуков), но без rateable его результат не используется.
  const feedback = useRecoFeedback();
  const rateScore = feedback.scoreFor;
  const rateable = Boolean(meta.rateable);
  const items = useMemo(() => {
    const all = data ?? [];
    // Дизлайкнутый трек исчезает из подборки сразу, не дожидаясь пересборки.
    return rateable ? all.filter((t) => rateScore(t) >= 0) : all;
  }, [data, rateable, rateScore]);

  const handlePlay = (track: Track) => {
    playTrack(track, items);
  };

  // Нет kind — подборка не выбрана.
  if (!kind) {
    return (
      <div className="flex flex-col gap-6 py-4">
        <PageHeader title="Коллекция" icon={Sparkles} />
        <EmptyState hint="Выберите коллекцию из навигации." />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 py-4">
      {/* Шапка подборки: название, описание, счётчик и запуск целиком */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-3xl font-bold text-white">{meta.title}</h1>
          {meta.subtitle && <p className="mt-1 text-slate-400">{meta.subtitle}</p>}
          <p className="mt-1 text-sm text-slate-500">
            {isLoading
              ? "Собираем подборку…"
              : pluralWithCount(items.length, "трек", "трека", "треков")}
            {!isLoading && rateable && items.length > 0 && (
              <> · оцените треки, и следующая подборка станет точнее</>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => items.length > 0 && playTrack(items[0], items)}
            disabled={items.length === 0}
            className="flex items-center gap-2 rounded-full bg-[#a855f7] px-6 py-3 font-semibold text-white shadow-lg shadow-[#a855f7]/40 transition-transform hover:scale-105 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:scale-100"
          >
            <Play size={20} fill="currentColor" />
            Слушать
          </button>
          <button
            onClick={() => void refetch()}
            disabled={isFetching}
            className="grid h-11 w-11 place-items-center rounded-full border border-white/10 bg-white/5 text-slate-300 transition-colors hover:text-white disabled:opacity-40"
            aria-label="Обновить подборку"
            title="Обновить подборку"
          >
            <RefreshCw size={18} className={isFetching ? "animate-spin" : undefined} />
          </button>
        </div>
      </div>

      {isLoading ? (
        <CollectionSkeleton />
      ) : error ? (
        <EmptyState
          hint="Не удалось загрузить подборку."
          action={{ label: "Попробовать снова", onClick: () => void refetch() }}
        />
      ) : items.length === 0 ? (
        <EmptyState hint={meta.emptyHint} />
      ) : (
        <div className="flex flex-col">
          {items.map((track, i) => (
            <TrackRow
              key={track.id}
              track={track}
              index={i}
              active={activeTrackId === track.id}
              playing={isPlaying}
              onPlay={handlePlay}
              showAlbum
              score={rateable ? rateScore(track) : undefined}
              onRate={rateable ? feedback.rate : undefined}
              rateDisabled={feedback.saving}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * Скелетон вместо текста «Загрузка…»: строки повторяют раскладку списка
 * (обложка 40×40 + две строки текста + длительность), поэтому при появлении
 * данных содержимое не «прыгает».
 */
function CollectionSkeleton() {
  return (
    <div className="flex flex-col gap-1">
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} className="flex animate-pulse items-center gap-4 rounded-xl px-3 py-2">
          <div className="h-10 w-10 shrink-0 rounded-lg bg-white/10" />
          <div className="min-w-0 flex-1">
            <div className="h-3.5 w-1/3 rounded bg-white/10" />
            <div className="mt-2 h-3 w-1/5 rounded bg-white/[0.07]" />
          </div>
          <div className="h-3 w-10 rounded bg-white/[0.07]" />
        </div>
      ))}
    </div>
  );
}

function EmptyState({
  hint,
  action,
}: {
  hint: string;
  action?: { label: string; onClick: () => void };
}) {
  return (
    <div className="grid place-items-center rounded-2xl border border-white/8 bg-white/5 px-6 py-16 text-center text-slate-400">
      <div className="max-w-md">
        <Sparkles size={40} className="mx-auto mb-3 opacity-40" />
        <p>{hint}</p>
        {action && (
          <button
            onClick={action.onClick}
            className="mt-4 rounded-xl border border-white/10 bg-white/5 px-4 py-2 text-sm text-white transition-colors hover:bg-white/10"
          >
            {action.label}
          </button>
        )}
      </div>
    </div>
  );
}

export default CollectionPage;
