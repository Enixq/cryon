import { useEffect, useRef } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "../lib/cn";

export interface TabItem<T extends string> {
  id: T;
  label: string;
  /** Необязательный значок слева от подписи (для настроек). */
  icon?: LucideIcon;
  /** Необязательный счётчик справа от подписи (для библиотеки/поиска). */
  count?: number;
}

interface TabBarProps<T extends string> {
  tabs: TabItem<T>[];
  active: T;
  onChange: (id: T) => void;
  /** Дополнительные классы для контейнера (например, отступы). */
  className?: string;
  /** Прижать вкладки к левому краю (по умолчанию) или растянуть на всю ширину. */
  stretch?: boolean;
}

/**
 * Подчёркнутая полоса вкладок в стиле макетов: активная вкладка выделяется
 * акцентным подчёркиванием (var(--app-accent)), а не «таблеткой». Единый
 * компонент для Поиска, Библиотеки и Настроек — раньше каждая страница верстала
 * вкладки собственными «таблетками» bg-white/8, из-за чего разделы выглядели
 * разнородно и не совпадали с макетами.
 */
export function TabBar<T extends string>({
  tabs,
  active,
  onChange,
  className,
  stretch = false,
}: TabBarProps<T>) {
  const ref = useRef<HTMLDivElement>(null);
  const suppressClick = useRef(0);
  useEffect(() => {
    const parent = ref.current?.parentElement;
    if (!parent) return;
    let start: { x: number; y: number; id: number; tabScroll?: number } | undefined;
    const down = (event: PointerEvent) => {
      if (
        event.pointerType !== "touch" ||
        !event.isPrimary ||
        (event.target as HTMLElement).closest(
          "input,select,textarea,[contenteditable=true],[role=slider],[data-no-tab-swipe]",
        )
      )
        return;
      const strip = (event.target as Element).closest("[role=tablist]");
      if (strip && strip !== ref.current) return;
      start = {
        x: event.clientX,
        y: event.clientY,
        id: event.pointerId,
        tabScroll: strip ? strip.scrollLeft : undefined,
      };
    };
    const move = (event: PointerEvent) => {
      if (!start || start.id !== event.pointerId || start.tabScroll === undefined || !ref.current)
        return;
      const dx = event.clientX - start.x,
        dy = event.clientY - start.y;
      // The page reserves horizontal gestures for tabs, so scrolling the tab
      // strip itself is handled here while vertical page scrolling stays native.
      if (Math.abs(dx) > 8 && Math.abs(dy) < Math.abs(dx)) {
        ref.current.scrollLeft = start.tabScroll - dx;
        suppressClick.current = Date.now() + 400;
        event.preventDefault();
      }
    };
    const up = (event: PointerEvent) => {
      if (!start || start.id !== event.pointerId) return;
      const dx = event.clientX - start.x,
        dy = event.clientY - start.y;
      const tabScroll = start.tabScroll;
      start = undefined;
      if (tabScroll !== undefined) return;
      if (Math.abs(dx) < 65 || Math.abs(dy) > Math.abs(dx) * 0.55) return;
      const index = tabs.findIndex((tab) => tab.id === active);
      const next = tabs[Math.max(0, Math.min(tabs.length - 1, index + (dx < 0 ? 1 : -1)))];
      if (next && next.id !== active) {
        event.preventDefault();
        suppressClick.current = Date.now() + 400;
        onChange(next.id);
      }
    };
    const cancel = () => {
      start = undefined;
    };
    const click = (event: MouseEvent) => {
      if (Date.now() < suppressClick.current) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    const original = parent.style.touchAction;
    const originalMinHeight = parent.style.minHeight;
    parent.style.touchAction = "pan-y pinch-zoom";
    // Keep empty sections swipeable in the centre of the page too.
    parent.style.minHeight = "100%";
    parent.addEventListener("pointerdown", down);
    parent.addEventListener("pointermove", move);
    parent.addEventListener("pointerup", up);
    parent.addEventListener("pointercancel", cancel);
    parent.addEventListener("click", click, true);
    return () => {
      parent.style.touchAction = original;
      parent.style.minHeight = originalMinHeight;
      parent.removeEventListener("pointerdown", down);
      parent.removeEventListener("pointermove", move);
      parent.removeEventListener("pointerup", up);
      parent.removeEventListener("pointercancel", cancel);
      parent.removeEventListener("click", click, true);
    };
  }, [tabs, active, onChange]);
  useEffect(() => {
    ref.current
      ?.querySelector<HTMLElement>("[aria-selected=true]")
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [active]);
  return (
    <div
      ref={ref}
      className={cn(
        // overflow-x-auto + no-scrollbar: на телефоне 4–5 вкладок не влезают по
        // ширине — раньше они распирали всю страницу в горизонтальный скролл
        // (баг адаптива в APK). Теперь лишние вкладки прокручиваются ВНУТРИ
        // полосы, а страница остаётся по ширине экрана. На десктопе вкладки
        // влезают — скролл не появляется.
        "flex gap-1 overflow-x-auto border-b border-white/8 no-scrollbar",
        className,
      )}
      role="tablist"
    >
      {tabs.map((t) => {
        const Icon = t.icon;
        const isActive = active === t.id;
        return (
          <button
            key={t.id}
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(t.id)}
            className={cn(
              "-mb-px flex items-center gap-2 whitespace-nowrap border-b-2 px-4 pb-3 pt-1 text-sm font-medium transition-colors",
              stretch ? "flex-1 justify-center" : "shrink-0",
              isActive
                ? "border-[var(--app-accent)] text-white"
                : "border-transparent text-slate-400 hover:text-white",
            )}
          >
            {Icon && <Icon size={16} />}
            <span>{t.label}</span>
            {t.count !== undefined && (
              <span
                className={cn(
                  "rounded-full px-1.5 py-0.5 text-[11px] tabular-nums transition-colors",
                  isActive
                    ? "bg-[color-mix(in_srgb,var(--app-accent)_28%,transparent)] text-white"
                    : "bg-white/8 text-slate-400",
                )}
              >
                {t.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
