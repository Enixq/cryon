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
export function TabBar<T extends string>({ tabs, active, onChange, className, stretch = false }: TabBarProps<T>) {
  return (
    <div className={cn("flex gap-1 border-b border-white/8", className)} role="tablist">
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
              "-mb-px flex items-center gap-2 border-b-2 px-4 pb-3 pt-1 text-sm font-medium transition-colors",
              stretch && "flex-1 justify-center",
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
                  isActive ? "bg-[color-mix(in_srgb,var(--app-accent)_28%,transparent)] text-white" : "bg-white/8 text-slate-400",
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
