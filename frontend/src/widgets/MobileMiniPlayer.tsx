import { useRef } from "react";
import type { TouchEvent } from "react";
import { Pause, Play, SkipBack, SkipForward } from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import { useUiStore } from "../store/uiStore";
import { Cover } from "../shared/ui/Cover";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { sourceName } from "../shared/sources";

export function MobileMiniPlayer() {
  const queue = usePlayerStore((s) => s.queue);
  const currentIndex = usePlayerStore((s) => s.currentIndex);
  const isPlaying = usePlayerStore((s) => s.isPlaying);
  const progress = usePlayerStore((s) => s.progress);
  const engineDuration = usePlayerStore((s) => s.engineDuration);
  const togglePlay = usePlayerStore((s) => s.togglePlay);
  const next = usePlayerStore((s) => s.next);
  const previous = usePlayerStore((s) => s.previous);
  const setNowPlayingOpen = useUiStore((s) => s.setNowPlayingOpen);
  const touchStartY = useRef<number | null>(null);
  const track = queue[currentIndex];
  const coverUrl = useTrackCover(track);

  if (!track) return null;
  const duration = engineDuration || track.duration || 0;
  const pct = duration > 0 ? Math.min(100, (progress / duration) * 100) : 0;

  function onTouchStart(event: TouchEvent<HTMLDivElement>) {
    if ((event.target as HTMLElement).closest("button")) return;
    touchStartY.current = event.touches[0]?.clientY ?? null;
  }

  function onTouchMove(event: TouchEvent<HTMLDivElement>) {
    const start = touchStartY.current;
    if (start === null) return;
    const current = event.touches[0]?.clientY ?? start;
    if (start - current > 48) {
      touchStartY.current = null;
      setNowPlayingOpen(true);
    }
  }

  function onTouchEnd() {
    touchStartY.current = null;
  }

  return (
    <div
      onTouchStart={onTouchStart}
      onTouchMove={onTouchMove}
      onTouchEnd={onTouchEnd}
      className="no-tap-highlight relative mx-2 mb-1 shrink-0 overflow-hidden rounded-2xl border border-white/8 bg-[var(--bg-2)]/95 shadow-lg backdrop-blur-xl"
    >
      <div className="absolute inset-x-0 top-0 h-0.5 bg-white/10">
        <div
          className="h-full transition-[width] duration-300"
          style={{ width: pct + "%", backgroundColor: "var(--app-accent)" }}
        />
      </div>
      <div className="flex items-center gap-2 p-2">
        <button
          type="button"
          data-noswipe
          onClick={() => setNowPlayingOpen(true)}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          aria-label="Открыть плеер"
        >
          <Cover
            accent={track.accent}
            src={coverUrl}
            alt={track.title}
            className="h-11 w-11 shrink-0"
            rounded="rounded-xl"
            iconSize={18}
          />
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-white">{track.title}</div>
            <div className="truncate text-xs text-slate-400">
              {track.artist} · {sourceName(track.source)}
            </div>
          </div>
        </button>
        <button
          type="button"
          data-noswipe
          onClick={togglePlay}
          className="grid h-10 w-10 shrink-0 place-items-center rounded-full text-white shadow-lg transition-transform active:scale-95"
          style={{ backgroundColor: "var(--app-accent)" }}
          aria-label={isPlaying ? "Пауза" : "Воспроизвести"}
        >
          {isPlaying ? (
            <Pause size={18} fill="currentColor" />
          ) : (
            <Play size={18} fill="currentColor" className="ml-0.5" />
          )}
        </button>
        <button
          type="button"
          data-noswipe
          onClick={previous}
          className="grid h-10 w-8 shrink-0 place-items-center rounded-full text-slate-300 transition-colors active:text-white"
          aria-label="Предыдущий трек"
        >
          <SkipBack size={17} fill="currentColor" />
        </button>
        <button
          type="button"
          data-noswipe
          onClick={next}
          className="grid h-10 w-8 shrink-0 place-items-center rounded-full text-slate-300 transition-colors active:text-white"
          aria-label="Следующий трек"
        >
          <SkipForward size={17} fill="currentColor" />
        </button>
      </div>
    </div>
  );
}
