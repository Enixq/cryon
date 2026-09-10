// Модальное окно эквалайзера: пресеты + 5 полос (60/230/910/3.6k/14k).
//
// Настройки живут в equalizerStore и применяются движком (useAudioEngine):
// mpv получает ffmpeg-фильтр, HTML5-фолбэк — Web Audio BiquadFilter. Здесь
// только UI — вся логика и хранение в сторе.

import { useEffect } from "react";
import { RotateCcw, SlidersHorizontal, X } from "lucide-react";
import {
  useEqualizerStore,
  EQ_FREQS,
  EQ_LABELS,
  EQ_MIN_DB,
  EQ_MAX_DB,
  EQ_PRESETS,
} from "../store/equalizerStore";
import { useUiStore } from "../store/uiStore";
import { cn } from "../shared/lib/cn";

function formatDb(v: number): string {
  if (v === 0) return "0";
  return v > 0 ? `+${v}` : `${v}`;
}

export function EqualizerModal() {
  const open = useUiStore((s) => s.equalizerOpen);
  const setOpen = useUiStore((s) => s.setEqualizerOpen);

  const enabled = useEqualizerStore((s) => s.enabled);
  const gains = useEqualizerStore((s) => s.gains);
  const preset = useEqualizerStore((s) => s.preset);
  const setEnabled = useEqualizerStore((s) => s.setEnabled);
  const setBand = useEqualizerStore((s) => s.setBand);
  const applyPreset = useEqualizerStore((s) => s.applyPreset);
  const reset = useEqualizerStore((s) => s.reset);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, setOpen]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm"
      onClick={() => setOpen(false)}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="equalizer-title"
        className="w-full max-w-md rounded-3xl border border-white/15 bg-[#10121e] p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-[color-mix(in_srgb,var(--app-accent)_25%,transparent)] text-[var(--app-accent)]">
              <SlidersHorizontal size={20} />
            </div>
            <div>
              <h2 id="equalizer-title" className="text-lg font-bold text-white">
                Эквалайзер
              </h2>
              <p className="text-xs text-slate-400">Тонкая настройка звучания</p>
            </div>
          </div>
          <button
            aria-label="Закрыть эквалайзер"
            onClick={() => setOpen(false)}
            className="text-slate-400 transition-colors hover:text-white"
          >
            <X size={20} />
          </button>
        </div>

        {/* Вкл/выкл */}
        <div className="mt-5 flex items-center justify-between rounded-2xl bg-white/[0.04] px-4 py-3">
          <span className="text-sm font-medium text-white">
            {enabled ? "Включён" : "Выключен"}
          </span>
          <button
            role="switch"
            aria-checked={enabled}
            aria-label="Включить эквалайзер"
            onClick={() => setEnabled(!enabled)}
            className={cn(
              "relative h-6 w-11 rounded-full transition-colors",
              enabled ? "bg-[var(--app-accent)]" : "bg-white/15",
            )}
          >
            <span
              className={cn(
                "absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform",
                enabled ? "translate-x-[22px]" : "translate-x-0.5",
              )}
            />
          </button>
        </div>

        {/* Пресеты */}
        <div className="mt-5 flex flex-wrap gap-2">
          {EQ_PRESETS.map((p) => (
            <button
              key={p.id}
              onClick={() => applyPreset(p.id)}
              className={cn(
                "rounded-full px-3 py-1.5 text-xs font-medium transition-colors",
                preset === p.id
                  ? "bg-[var(--app-accent)] text-slate-950"
                  : "bg-white/[0.06] text-slate-300 hover:bg-white/10 hover:text-white",
              )}
            >
              {p.name}
            </button>
          ))}
          {preset === "custom" && (
            <span className="rounded-full bg-white/[0.06] px-3 py-1.5 text-xs font-medium text-[var(--app-accent)]">
              Свой
            </span>
          )}
        </div>

        {/* Полосы */}
        <div
          className={cn(
            "mt-6 flex items-end justify-between gap-2 transition-opacity",
            !enabled && "opacity-50",
          )}
        >
          {EQ_FREQS.map((freq, i) => (
            <div key={freq} className="flex flex-1 flex-col items-center gap-2">
              <span className="text-xs font-semibold tabular-nums text-slate-300">
                {formatDb(gains[i] ?? 0)}
              </span>
              <input
                type="range"
                min={EQ_MIN_DB}
                max={EQ_MAX_DB}
                step={1}
                value={gains[i] ?? 0}
                onChange={(e) => setBand(i, Number(e.target.value))}
                aria-label={`Полоса ${EQ_LABELS[i]} Гц`}
                // Явная центрированная ширина: нативный вертикальный range без
                // width брал разную ширину в разных движках и мог смотреться
                // смещённым в колонке. w-6 + mx-auto держат ползунок по центру.
                className="mx-auto h-36 w-6 cursor-pointer"
                style={{
                  writingMode: "vertical-lr",
                  direction: "rtl",
                  accentColor: "var(--app-accent)",
                }}
              />
              <span className="text-[11px] tabular-nums text-slate-500">{EQ_LABELS[i]}</span>
            </div>
          ))}
        </div>

        {/* Сброс */}
        <div className="mt-6 flex justify-end">
          <button
            onClick={reset}
            className="flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-slate-300 transition-colors hover:bg-white/5 hover:text-white"
          >
            <RotateCcw size={15} />
            Сбросить
          </button>
        </div>
      </section>
    </div>
  );
}
