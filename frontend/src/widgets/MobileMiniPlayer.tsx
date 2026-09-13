import { useRef, useState } from "react";
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
  const drag = useRef<{ x: number; y: number; active: boolean; distance: number } | null>(null);
  const [pullUp, setPullUp] = useState(0);
  const track = queue[currentIndex];
  const coverUrl = useTrackCover(track);

  if (!track) return null;
  const duration = engineDuration || track.duration || 0;
  const percent = duration > 0 ? Math.min(100, (progress / duration) * 100) : 0;

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if ((event.target as HTMLElement).closest("button")) return;
    drag.current = { x: event.clientX, y: event.clientY, active: false, distance: 0 };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const state = drag.current;
    if (!state) return;
    const deltaY = state.y - event.clientY;
    const deltaX = Math.abs(event.clientX - state.x);
    if (!state.active) {
      if (Math.abs(deltaY) < 8 && deltaX < 8) return;
      if (deltaY <= 0 || deltaY <= deltaX) {
        drag.current = null;
        return;
      }
      state.active = true;
    }
    state.distance = Math.min(96, Math.max(0, deltaY));
    setPullUp(state.distance);
  }

  function finishDrag() {
    const state = drag.current;
    drag.current = null;
    setPullUp(0);
    if (state?.active && state.distance > 44) setNowPlayingOpen(true);
  }

  return (
    <div
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={finishDrag}
      onPointerCancel={finishDrag}
      style={{ touchAction: "none", transform: pullUp ? "translateY(-" + pullUp * 0.2 + "px)" : undefined }}
      className="no-tap-highlight relative mx-2 mb-1 shrink-0 overflow-hidden rounded-2xl bg-[var(--bg-2)]/95 shadow-lg shadow-black/20 backdrop-blur-xl transition-transform"
    >
      <div className="absolute inset-x-0 top-0 h-0.5 bg-white/10"><div className="h-full transition-[width] duration-300" style={{ width: percent + "%", backgroundColor: "var(--app-accent)" }} /></div>
      <div className="flex items-center gap-1.5 p-1.5">
        <button type="button" onClick={() => setNowPlayingOpen(true)} className="flex min-w-0 flex-1 items-center gap-2 text-left outline-none" aria-label="Открыть плеер">
          <Cover accent={track.accent} src={coverUrl} alt={track.title} className="h-10 w-10 shrink-0" rounded="rounded-xl" iconSize={17} />
          <div className="min-w-0"><div className="truncate text-[13px] font-semibold text-white">{track.title}</div><div className="truncate text-[11px] text-slate-400">{track.artist} · {sourceName(track.source)}</div></div>
        </button>
        <button type="button" onClick={previous} className="grid h-9 w-7 shrink-0 place-items-center rounded-full text-slate-300 active:text-white" aria-label="Предыдущий трек"><SkipBack size={16} fill="currentColor" /></button>
        <button type="button" onClick={togglePlay} className="grid h-9 w-9 shrink-0 place-items-center rounded-full text-white shadow-lg active:scale-95" style={{ backgroundColor: "var(--app-accent)" }} aria-label={isPlaying ? "Пауза" : "Воспроизвести"}>{isPlaying ? <Pause size={17} fill="currentColor" /> : <Play size={17} fill="currentColor" className="ml-0.5" />}</button>
        <button type="button" onClick={next} className="grid h-9 w-7 shrink-0 place-items-center rounded-full text-slate-300 active:text-white" aria-label="Следующий трек"><SkipForward size={16} fill="currentColor" /></button>
      </div>
    </div>
  );
}
