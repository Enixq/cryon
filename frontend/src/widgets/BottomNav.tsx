import { Home, Library, LayoutGrid, Search, User } from "lucide-react";
import { NavLink } from "react-router-dom";
import { cn } from "../shared/lib/cn";

/**
 * Нижняя навигация из 5 пунктов — мобильный аналог боковой панели
 * ([Sidebar](Sidebar.tsx)). Показывается только на телефонной ширине
 * (см. ветку isPhone в [AppLayout](AppLayout.tsx)). Активный пункт
 * подсвечивается «таблеткой» и акцентом текущей обложки (var(--app-accent)),
 * как активные пункты в сайдбаре. Тап-таргеты ≥ 56px по высоте.
 */
const items = [
  { to: "/", label: "Главная", icon: Home, end: true },
  { to: "/search", label: "Поиск", icon: Search },
  { to: "/library", label: "Библиотека", icon: Library },
  { to: "/services", label: "Сервисы", icon: LayoutGrid },
  { to: "/settings", label: "Профиль", icon: User },
];

export function BottomNav() {
  return (
    <nav className="no-tap-highlight pb-safe shrink-0 border-t border-white/5 bg-[var(--bg-1)]/95 backdrop-blur-xl">
      <div className="flex items-stretch">
        {items.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                "flex flex-1 flex-col items-center justify-center gap-1 pb-1.5 pt-2 text-[11px] font-medium transition-colors",
                isActive ? "text-white" : "text-slate-500 hover:text-slate-300",
              )
            }
          >
            {({ isActive }) => (
              <>
                <span
                  className="grid h-8 w-14 place-items-center rounded-full transition-colors"
                  style={
                    isActive
                      ? {
                          backgroundColor: "color-mix(in srgb, var(--app-accent) 22%, transparent)",
                          color: "var(--app-accent)",
                        }
                      : undefined
                  }
                >
                  <Icon size={20} />
                </span>
                <span style={isActive ? { color: "var(--app-accent)" } : undefined}>{label}</span>
              </>
            )}
          </NavLink>
        ))}
      </div>
    </nav>
  );
}
