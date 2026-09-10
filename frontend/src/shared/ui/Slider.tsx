import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type WheelEvent as ReactWheelEvent,
} from "react";
import { cn } from "../lib/cn";
import { clamp } from "../lib/format";

interface SliderProps {
  /** Текущее значение. */
  value: number;
  min?: number;
  max: number;
  step?: number;
  /** Вызывается во время перетаскивания и по клику. */
  onChange: (value: number) => void;
  /** Вызывается по окончании перетаскивания (commit). */
  onCommit?: (value: number) => void;
  /** Шаг изменения колесом мыши. Если 0 — колесо отключено. */
  wheelStep?: number;
  className?: string;
  ariaLabel?: string;
  /** Компактный вид (тоньше дорожка). */
  compact?: boolean;
}

/**
 * Интерактивный слайдер: клик по дорожке, перетаскивание мышью и колесо мыши.
 * Используется для громкости и полоски времени трека.
 */
export function Slider({
  value,
  min = 0,
  max,
  step = 1,
  onChange,
  onCommit,
  wheelStep = 0,
  className,
  ariaLabel,
  compact = false,
}: SliderProps) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState(false);

  const range = max - min || 1;
  const percent = clamp(((value - min) / range) * 100, 0, 100);

  const valueFromClientX = useCallback(
    (clientX: number): number => {
      const el = trackRef.current;
      if (!el) return value;
      const rect = el.getBoundingClientRect();
      const ratio = clamp((clientX - rect.left) / rect.width, 0, 1);
      let raw = min + ratio * range;
      if (step > 0) raw = Math.round(raw / step) * step;
      return clamp(raw, min, max);
    },
    [min, max, range, step, value],
  );

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(true);
    onChange(valueFromClientX(event.clientX));
  };

  // Глобальные слушатели во время перетаскивания, чтобы drag не срывался за пределами дорожки.
  useEffect(() => {
    if (!dragging) return;

    const handleMove = (event: PointerEvent) => {
      onChange(valueFromClientX(event.clientX));
    };
    const handleUp = (event: PointerEvent) => {
      setDragging(false);
      const next = valueFromClientX(event.clientX);
      onChange(next);
      onCommit?.(next);
    };

    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
    return () => {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
    };
  }, [dragging, valueFromClientX, onChange, onCommit]);

  const handleWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (!wheelStep) return;
    event.preventDefault();
    const delta = event.deltaY < 0 ? wheelStep : -wheelStep;
    const next = clamp(value + delta, min, max);
    onChange(next);
    onCommit?.(next);
  };

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const stepSize = wheelStep || step || 1;
    let next = value;
    if (event.key === "ArrowRight" || event.key === "ArrowUp") next = value + stepSize;
    else if (event.key === "ArrowLeft" || event.key === "ArrowDown") next = value - stepSize;
    else if (event.key === "Home") next = min;
    else if (event.key === "End") next = max;
    else return;
    event.preventDefault();
    next = clamp(next, min, max);
    onChange(next);
    onCommit?.(next);
  };

  return (
    <div
      ref={trackRef}
      role="slider"
      tabIndex={0}
      aria-label={ariaLabel}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={Math.round(value)}
      onPointerDown={handlePointerDown}
      onWheel={handleWheel}
      onKeyDown={handleKeyDown}
      className={cn(
        // ВАЖНО: центрирование по вертикали делаем НЕ через `flex items-center`
        // на корне, а абсолютным позиционированием дорожки/ползунка ниже. Иначе
        // достаточно вызывающему передать display-класс (например, `lg:block`
        // у громкости в PlayerBar), и он перебивает `flex` — дорожка уезжает
        // вверх, а ползунок оказывается ниже центра. Теперь корню нужен только
        // `relative` и высота как область попадания.
        "group relative cursor-pointer",
        compact ? "h-3" : "h-4",
        className,
      )}
    >
      {/* Дорожка — абсолютно по центру корня, поэтому выравнивание не зависит
          от display вызывающего блока. */}
      <div
        className={cn(
          "absolute inset-x-0 top-1/2 -translate-y-1/2 overflow-hidden rounded-full bg-white/12",
          compact ? "h-1" : "h-1.5",
        )}
      >
        {/* Заполнение — цвет следует активному акценту приложения. */}
        <div
          className="h-full rounded-full"
          style={{
            width: `${percent}%`,
            background:
              "linear-gradient(to right, color-mix(in srgb, var(--app-accent) 70%, #ffffff 0%), var(--app-accent))",
          }}
        />
      </div>
      {/* Ползунок — свечение тоже от акцента. */}
      <div
        className={cn(
          "absolute top-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full bg-white transition-transform",
          dragging ? "scale-110" : "scale-0 group-hover:scale-100",
          compact ? "h-3 w-3" : "h-3.5 w-3.5",
        )}
        style={{
          left: `${percent}%`,
          boxShadow: "0 0 10px color-mix(in srgb, var(--app-accent) 80%, transparent)",
        }}
      />
    </div>
  );
}
