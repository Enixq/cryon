import {
  Clock3,
  HardDrive,
  Heart,
  Home,
  Library,
  ListMusic,
  Plus,
  Search,
  Settings,
  ChevronDown,
} from "lucide-react";
import { NavLink, useNavigate } from "react-router-dom";
import { useEffect, useState } from "react";
import { cn } from "../shared/lib/cn";
import { SOURCES } from "../shared/sources";
import { listSourceStatus, onSourceStatusChanged } from "../shared/api/client";
import type { SourceLevel } from "../shared/api/client";
import type { SourceId } from "../shared/types";
import { LogoFull, LogoMark } from "../shared/ui/Logo";
import { ServiceIcon } from "../shared/ui/ServiceIcon";
import { useUiStore } from "../store/uiStore";
import { useMediaQuery, LAYOUT_BREAKPOINTS } from "../shared/lib/useMediaQuery";

// Цвет и подпись индикатора статуса источника.
const STATUS_META: Record<SourceLevel, { color: string; title: string }> = {
  ok: { color: "bg-emerald-400", title: "Подключено, играет напрямую" },
  limited: { color: "bg-amber-400", title: "Ограниченно: только поиск/внешние ссылки" },
  off: { color: "bg-rose-500", title: "Не подключено" },
};

const navItems = [
  { to: "/", label: "Главная", icon: Home, end: true },
  { to: "/search", label: "Поиск", icon: Search },
  { to: "/library", label: "Библиотека", icon: Library },
  { to: "/local", label: "Локальная музыка", icon: HardDrive },
  { to: "/playlists", label: "Плейлисты", icon: ListMusic },
  { to: "/favorites", label: "Избранное", icon: Heart },
  { to: "/history", label: "История", icon: Clock3 },
];

// Сервисы, отображаемые в блоке "Мои сервисы".
const serviceOrder: SourceId[] = ["spotify", "soundcloud", "vk", "yandex", "youtube", "local"];

export function Sidebar() {
  const navigate = useNavigate();
  // Реальное состояние подключения источников из backend (не мок).
  const [levels, setLevels] = useState<Record<string, SourceLevel>>({});

  // Адаптив (задача 29): на узком экране панель принудительно сворачивается
  // в значковый рельс; на широком уважаем ручной флаг из топбара.
  const forceCollapse = useMediaQuery(LAYOUT_BREAKPOINTS.collapseSidebar);
  const manualCollapsed = useUiStore((s) => s.sidebarCollapsed);
  const collapsed = forceCollapse || manualCollapsed;

  useEffect(() => {
    let alive = true;
    const refresh = () => {
      void listSourceStatus().then((rows) => {
        if (!alive) return;
        setLevels(Object.fromEntries(rows.map((r) => [r.id, r.level])));
      });
    };
    refresh();
    const off = onSourceStatusChanged(refresh);
    return () => {
      alive = false;
      off();
    };
  }, []);

  return (
    <aside
      className={cn(
        "flex shrink-0 flex-col gap-6 border-r border-white/5 bg-[#0c0e1a]/60 py-5 transition-[width] duration-200",
        collapsed ? "w-[68px] items-center px-2" : "w-[248px] px-4",
      )}
    >
      {/* Логотип */}
      {collapsed ? <LogoMark size={36} className="mx-auto" /> : <LogoFull className="px-2" size={36} />}

      {/* Навигация */}
      <nav className={cn("flex flex-col gap-1", collapsed && "w-full items-center")}>
        {navItems.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            title={collapsed ? label : undefined}
            className={({ isActive }) =>
              cn(
                "flex items-center rounded-xl text-[15px] font-medium transition-colors",
                collapsed ? "h-11 w-11 justify-center" : "gap-3 px-3 py-2.5",
                isActive
                  ? "text-white"
                  : "text-slate-400 hover:bg-white/5 hover:text-white",
              )
            }
            style={({ isActive }) => (isActive ? { backgroundColor: "var(--app-accent)" } : undefined)}
          >
            <Icon size={20} />
            {!collapsed && <span>{label}</span>}
          </NavLink>
        ))}
      </nav>

      {/* Мои сервисы */}
      <div className={cn("flex flex-col gap-2", collapsed && "w-full items-center")}>
        {!collapsed && (
          <div className="flex items-center justify-between px-3">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-500">
              Мои сервисы
            </span>
            <button type="button" onClick={() => navigate("/settings")} className="text-slate-500 transition-colors hover:text-white" aria-label="Настроить сервисы" title="Настроить сервисы">
              <Plus size={16} />
            </button>
          </div>
        )}
        <div className={cn("flex flex-col gap-0.5", collapsed && "items-center")}>
          {serviceOrder.map((id) => {
            const meta = SOURCES[id];
            const level = levels[id] ?? "off";
            const status = STATUS_META[level];
            if (collapsed) {
              return (
                <button
                  key={id}
                  type="button"
                  onClick={() => navigate("/settings")}
                  className="relative grid h-11 w-11 place-items-center rounded-lg transition-colors hover:bg-white/5"
                  title={`${meta.name} — ${status.title}`}
                >
                  <ServiceIcon id={id} size={24} badge />
                  <span
                    className={cn("absolute right-1.5 top-1.5 h-2 w-2 rounded-full ring-2 ring-[#0c0e1a]", status.color)}
                    aria-label={status.title}
                  />
                </button>
              );
            }
            return (
              <button
                key={id}
                type="button"
                onClick={() => navigate("/settings")}
                className="flex items-center gap-3 rounded-lg px-3 py-2 text-[15px] text-slate-300 transition-colors hover:bg-white/5"
                title={`Настроить ${meta.name}`}
              >
                <ServiceIcon id={id} size={24} badge />
                <span className="flex-1">{meta.name}</span>
                <span
                  className={cn("h-2 w-2 rounded-full", status.color)}
                  title={status.title}
                  aria-label={status.title}
                />
              </button>
            );
          })}
        </div>
      </div>

      {/* Нижняя часть: настройки + профиль. Раньше здесь были ещё иконки
          «Оформление» (рубашка) и «Тема» (луна), но обе вели в те же настройки,
          а обещанных ими экранов нет — иконки вводили в заблуждение, поэтому
          убраны. Осталась одна честная кнопка: шестерёнка → настройки. */}
      <div className={cn("mt-auto flex flex-col gap-4", collapsed && "w-full items-center")}>
        <div className={cn("flex items-center gap-4 text-slate-400", collapsed ? "flex-col" : "px-3")}>
          <NavLink to="/settings" className="transition-colors hover:text-white" aria-label="Настройки" title="Настройки">
            <Settings size={18} />
          </NavLink>
        </div>

        {collapsed ? (
          <button
            type="button"
            onClick={() => navigate("/settings")}
            className="grid h-10 w-10 place-items-center rounded-xl bg-white/5 transition-colors hover:bg-white/10"
            title="Alex — Premium"
            aria-label="Профиль: Alex, Premium"
          >
            <div
              className="h-8 w-8 rounded-lg"
              style={{ background: "linear-gradient(135deg, #ec4899, #8b5cf6)" }}
            />
          </button>
        ) : (
          <button type="button" onClick={() => navigate("/settings")} className="flex items-center gap-3 rounded-xl bg-white/5 px-3 py-2.5 text-left transition-colors hover:bg-white/10">
            <div
              className="h-9 w-9 rounded-lg"
              style={{ background: "linear-gradient(135deg, #ec4899, #8b5cf6)" }}
            />
            <div className="flex-1">
              <div className="text-sm font-semibold text-white">Alex</div>
              <div className="text-xs text-[#c084fc]">Premium</div>
            </div>
            <ChevronDown size={16} className="text-slate-400" />
          </button>
        )}
      </div>
    </aside>
  );
}
