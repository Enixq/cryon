import {
  Heart,
  ListMusic,
  Pause,
  Play,
  Radio,
  Repeat,
  Repeat1,
  Shuffle,
  SkipBack,
  SkipForward,
  SlidersHorizontal,
  Volume1,
  Volume2,
  VolumeX,
} from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import { Slider } from "../shared/ui/Slider";
import { Cover } from "../shared/ui/Cover";
import { formatDuration } from "../shared/lib/format";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import { TrackQuickActions } from "../shared/ui/TrackQuickActions";
import { ArtistLink } from "../shared/ui/ArtistLink";
import { useUiStore } from "../store/uiStore";
import { useEqualizerStore } from "../store/equalizerStore";
import { LAYOUT_BREAKPOINTS, useMediaQuery } from "../shared/lib/useMediaQuery";

/**
 * Нижняя панель воспроизведения. Дублирует ключевые контролы плеера,
 * всегда видима на всех страницах. Колесо мыши над блоком громкости
 * изменяет громкость.
 */
export function PlayerBar() {
  const {
    queue,
    currentIndex,
    isPlaying,
    progress,
    volume,
    muted,
    shuffle,
    repeat,
    radio,
    togglePlay,
    next,
    previous,
    seek,
    setVolume,
    toggleMute,
    toggleShuffle,
    cycleRepeat,
    toggleRadio,
    toggleLikeWithTrack,
  } = usePlayerStore();

  const track = queue[currentIndex];
  const favoriteIds = useFavoriteIds();
  const isFavorite = track ? favoriteIds.has(track.id) : false;
  const effectiveVolume = muted ? 0 : volume;
  const coverUrl = useTrackCover(track);
  const engineDuration = usePlayerStore((s) => s.engineDuration);
  const setNowPlayingOpen = useUiStore((s) => s.setNowPlayingOpen);
  const setEqualizerOpen = useUiStore((s) => s.setEqualizerOpen);
  const nowPlayingCollapsed = useUiStore((s) => s.nowPlayingCollapsed);
  const toggleNowPlayingCollapsed = useUiStore((s) => s.toggleNowPlayingCollapsed);
  const eqEnabled = useEqualizerStore((s) => s.enabled);
  const isNarrow = useMediaQuery(LAYOUT_BREAKPOINTS.hideNowPlaying);

  const openQueue = () => {
    if (isNarrow) {
      setNowPlayingOpen(true);
      return;
    }
    // Широкий экран: панель «Сейчас играет» встроена справа, поэтому кнопка
    // прячет/показывает её (а не скроллит к уже видимой очереди, как раньше —
    // тогда казалось, что кнопка ничего не делает).
    toggleNowPlayingCollapsed();
  };

  return (
    <footer className="flex h-[88px] shrink-0 items-center gap-3 border-t border-white/5 bg-[#0c0e1a]/90 px-3 backdrop-blur-xl sm:gap-6 sm:px-6">
      {/* Слева: текущий трек. На узких экранах ширина ужимается, чтобы центр
          с транспортом не сжимался и не перекрывался. */}
      <div className="flex w-[160px] items-center gap-3 lg:w-[280px]">
        {track ? (
          <>
            <Cover accent={track.accent} src={coverUrl} alt={track.title} className="h-14 w-14 shrink-0" iconSize={22} />
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-white">{track.title}</div>
              <div className="truncate text-xs text-slate-400">
                <ArtistLink name={track.artist} /> · {sourceName(track.source)}
              </div>
            </div>
            <button
              onClick={() => toggleLikeWithTrack({ ...track, liked: isFavorite })}
              className={cn("shrink-0 transition-colors", isFavorite ? "text-[#a855f7]" : "text-slate-400 hover:text-white")}
              aria-label="В избранное"
            >
              <Heart size={18} fill={isFavorite ? "currentColor" : "none"} />
            </button>
            <TrackQuickActions track={track} />
          </>
        ) : (
          <div className="text-sm text-slate-500">Ничего не играет</div>
        )}
      </div>

      {/* Центр: транспорт + полоска времени */}
      <div className="flex flex-1 flex-col items-center gap-2">
        <div className="flex items-center gap-5">
          <button
            onClick={toggleShuffle}
            className={cn("transition-colors", shuffle ? "text-white" : "text-slate-400 hover:text-white")}
            style={shuffle ? { color: "var(--app-accent)" } : undefined}
            aria-label="Перемешать"
          >
            <Shuffle size={18} />
          </button>
          <button onClick={previous} className="text-slate-200 transition-transform hover:scale-110 hover:text-white" aria-label="Предыдущий">
            <SkipBack size={20} fill="currentColor" />
          </button>
          <button
            onClick={togglePlay}
            className="grid h-11 w-11 place-items-center rounded-full text-white shadow-lg transition-transform hover:scale-105"
            style={{ backgroundColor: "var(--app-accent)", boxShadow: "0 12px 30px -10px var(--app-accent)" }}
            aria-label={isPlaying ? "Пауза" : "Воспроизвести"}
          >
            {isPlaying ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" className="ml-0.5" />}
          </button>
          <button onClick={next} className="text-slate-200 transition-transform hover:scale-110 hover:text-white" aria-label="Следующий">
            <SkipForward size={20} fill="currentColor" />
          </button>
          <button
            onClick={cycleRepeat}
            className={cn("transition-colors", repeat !== "off" ? "text-white" : "text-slate-400 hover:text-white")}
            style={repeat !== "off" ? { color: "var(--app-accent)" } : undefined}
            aria-label="Повтор"
          >
            {repeat === "one" ? <Repeat1 size={18} /> : <Repeat size={18} />}
          </button>
          <button
            onClick={toggleRadio}
            className={cn("transition-colors", radio ? "text-white" : "text-slate-400 hover:text-white")}
            style={radio ? { color: "var(--app-accent)" } : undefined}
            aria-label="Радио: продолжать похожими треками"
            title="Радио: когда очередь закончится, продолжить похожими треками"
          >
            <Radio size={18} />
          </button>
        </div>

        <div className="flex w-full max-w-2xl items-center gap-3">
          <span className="w-10 text-right text-xs tabular-nums text-slate-400">
            {formatDuration(progress)}
          </span>
          <Slider
            value={progress}
            max={track ? engineDuration || track.duration : 100}
            onChange={seek}
            ariaLabel="Полоска времени воспроизведения"
            className="flex-1"
            compact
          />
          <span className="w-10 text-xs tabular-nums text-slate-400">
            {track ? formatDuration(engineDuration || track.duration) : "0:00"}
          </span>
        </div>
      </div>

      {/* Справа: громкость + очередь. На узких экранах ползунок громкости
          скрыт (кнопка mute/колесо мыши остаются), чтобы освободить место
          центральному транспорту. */}
      <div className="flex w-[64px] items-center justify-end gap-3 lg:w-[220px]">
        <button onClick={toggleMute} className="text-slate-400 transition-colors hover:text-white" aria-label="Без звука">
          {muted || volume === 0 ? <VolumeX size={18} /> : volume < 50 ? <Volume1 size={18} /> : <Volume2 size={18} />}
        </button>
        <Slider
          value={effectiveVolume}
          max={100}
          onChange={setVolume}
          wheelStep={4}
          ariaLabel="Полоска громкости"
          className="hidden w-28 lg:flex"
          compact
        />
        <button
          onClick={() => setEqualizerOpen(true)}
          className={cn(
            "hidden transition-colors sm:block",
            eqEnabled ? "text-[var(--app-accent)]" : "text-slate-400 hover:text-white",
          )}
          aria-label="Эквалайзер"
          title="Эквалайзер"
        >
          <SlidersHorizontal size={18} />
        </button>
        <button
          onClick={openQueue}
          className={cn(
            "hidden transition-colors sm:block",
            !isNarrow && !nowPlayingCollapsed ? "text-[var(--app-accent)]" : "text-slate-400 hover:text-white",
          )}
          aria-label={isNarrow ? "Открыть очередь" : nowPlayingCollapsed ? "Показать панель «Сейчас играет»" : "Скрыть панель «Сейчас играет»"}
          title={isNarrow ? "Очередь" : nowPlayingCollapsed ? "Показать «Сейчас играет» и очередь" : "Скрыть «Сейчас играет» и очередь"}
        >
          <ListMusic size={18} />
        </button>
      </div>
    </footer>
  );
}
