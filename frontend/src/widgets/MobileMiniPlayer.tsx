import { useRef } from "react";
import type { PointerEvent } from "react";
import { Pause, Play, SkipBack, SkipForward } from "lucide-react";
import { usePlayerStore } from "../store/playerStore";
import { useUiStore } from "../store/uiStore";
import { Cover } from "../shared/ui/Cover";
import { useTrackCover } from "../shared/lib/useTrackCover";
import { sourceName } from "../shared/sources";

export function MobileMiniPlayer() {
  const queue = usePlayerStore((state) => state.queue);
  const currentIndex = usePlayerStore((state) => state.currentIndex);
  const isPlaying = usePlayerStore((state) => state.isPlaying);
  const progress = usePlayerStore((state) => state.progress);
  const engineDuration = usePlayerStore((state) => state.engineDuration);
  const togglePlay = usePlayerStore((state) => state.togglePlay);
  const next = usePlayerStore((state) => state.next);
  const previous = usePlayerStore((state) => state.previous);
  const setNowPlayingOpen = useUiStore((state) => state.setNowPlayingOpen);
  const swipeStart = useRef<{ x: number; y: number } | null>(null);
  const track = queue[currentIndex];
  const coverUrl = useTrackCover(track);

  if (!track) return null;
  const duration = engineDuration || track.duration || 0;
  const percent = duration > 0 ? Math.min(100, (progress / duration) * 100) : 0;

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    swipeStart.current = { x: event.clientX, y: event.clientY };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const start = swipeStart.current;
    if (!start) return;
    const verticalDistance = start.y - event.clientY;
    const horizontalDistance = Math.abs(event.clientX - start.x);
    if (verticalDistance > 36 && verticalDistance > horizontalDistance) {
      swipeStart.current = null;
      setNowPlayingOpen(true);
    }
  }

  return (
    <div onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={() => { swipeStart.current = null; }} onPointerCancel={() => { swipeStart.current = null; }} style={{ touchAction: "none" }} className="no-tap-highlight relative mx-2 mb-1 shrink-0 overflow-hidden rounded-2xl border border-white/8 bg-[var(--bg-2)]/95 shadow-lg backdrop-blur-xl">
      <div className="absolute inset-x-0 top-0 h-0.5 bg-white/10"><div className="h-full transition-[width] duration-300" style={{ width: percent + "%", backgroundColor: "var(--app-accent)" }} /></div>
      <div className="flex items-center gap-2 p-2">
        <button type="button" onClick={() => setNowPlayingOpen(true)} className="flex min-w-0 flex-1 items-center gap-2 text-left" aria-label="Открыть плеер">
          <Cover accent={track.accent} src={coverUrl} alt={track.title} className="h-11 w-11 shrink-0" rounded="rounded-xl" iconSize={18} />
          <div className="min-w-0"><div className="truncate text-sm font-semibold text-white">{track.title}</div><div className="truncate text-xs text-slate-400">{track.artist} · {sourceName(track.source)}</div></div>
        </button>
        <button type="button" onClick={previous} className="grid h-10 w-8 shrink-0 place-items-center rounded-full text-slate-300 active:text-white" aria-label="Предыдущий трек"><SkipBack size={17} fill="currentColor" /></button>
        <button type="button" onClick={togglePlay} className="grid h-10 w-10 shrink-0 place-items-center rounded-full text-white shadow-lg active:scale-95" style={{ backgroundColor: "var(--app-accent)" }} aria-label={isPlaying ? "Пауза" : "Воспроизвести"}>{isPlaying ? <Pause size={18} fill="currentColor" /> : <Play size={18} fill="currentColor" className="ml-0.5" />}</button>
        <button type="button" onClick={next} className="grid h-10 w-8 shrink-0 place-items-center rounded-full text-slate-300 active:text-white" aria-label="Следующий трек"><SkipForward size={17} fill="currentColor" /></button>
      </div>
    </div>
  );
}
