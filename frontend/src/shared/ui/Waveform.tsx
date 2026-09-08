import { useMemo, useRef } from "react";

/**
 * Waveform — визуализация звуковой дорожки в виде столбиков.
 *
 * Реальный поток играет mpv в бэкенде, и WebView не имеет доступа к его
 * аудиоданным для честного FFT-анализа. Поэтому форма волны генерируется
 * детерминированно из id трека: у каждого трека своя стабильная «картинка»,
 * которая заполняется по мере воспроизведения. Клик по волне перематывает.
 */
export function Waveform({
  seed,
  progress,
  duration,
  onSeek,
  bars = 64,
  fillColor = "#a855f7",
  className,
}: {
  seed: string;
  progress: number;
  duration: number;
  onSeek: (positionS: number) => void;
  bars?: number;
  fillColor?: string;
  className?: string;
}) {
  const containerRef = useRef<HTMLDivElement>(null);

  // Высоты столбиков детерминированы по seed — не «прыгают» между рендерами.
  const heights = useMemo(() => generateHeights(seed, bars), [seed, bars]);

  const ratio = duration > 0 ? Math.min(1, Math.max(0, progress / duration)) : 0;
  const filledCount = Math.round(ratio * bars);

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (duration <= 0) return;
    const el = containerRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    onSeek(Math.min(1, Math.max(0, x)) * duration);
  };

  return (
    <div
      ref={containerRef}
      onClick={handleClick}
      role="slider"
      aria-label="Форма волны, перемотка"
      aria-valuemin={0}
      aria-valuemax={Math.round(duration)}
      aria-valuenow={Math.round(progress)}
      tabIndex={0}
      className={
        "flex h-10 cursor-pointer items-center gap-[2px] " + (className ?? "")
      }
    >
      {heights.map((h, i) => (
        <span
          key={i}
          className="flex-1 rounded-full transition-colors"
          style={{
            height: `${h}%`,
            backgroundColor: i < filledCount ? fillColor : "rgba(255,255,255,0.14)",
          }}
        />
      ))}
    </div>
  );
}

// generateHeights строит псевдослучайные, но стабильные высоты столбиков
// из строки-seed. Небольшая синусоидальная огибающая делает форму «живой».
function generateHeights(seed: string, bars: number): number[] {
  let state = 0;
  for (let i = 0; i < seed.length; i++) {
    state = (state * 31 + seed.charCodeAt(i)) >>> 0;
  }
  const rand = () => {
    // xorshift32 — быстрый детерминированный ГПСЧ.
    state ^= state << 13;
    state ^= state >>> 17;
    state ^= state << 5;
    state >>>= 0;
    return state / 0xffffffff;
  };

  const out: number[] = [];
  for (let i = 0; i < bars; i++) {
    const envelope = 0.5 + 0.5 * Math.sin((i / bars) * Math.PI);
    const value = (0.25 + rand() * 0.75) * envelope;
    out.push(Math.round(20 + value * 80));
  }
  return out;
}
