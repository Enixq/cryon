import { Bell, Check, Info, TriangleAlert, X, CircleCheck, CircleX } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import {
  useNotificationStore,
  selectUnreadCount,
  type NotificationKind,
} from "../store/notificationStore";
import { cn } from "../shared/lib/cn";

const KIND_ICON: Record<NotificationKind, typeof Info> = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  error: CircleX,
};

const KIND_COLOR: Record<NotificationKind, string> = {
  info: "text-sky-400",
  success: "text-emerald-400",
  warning: "text-amber-400",
  error: "text-rose-400",
};

/** Относительное время: «только что», «5 мин назад», «2 ч назад», иначе дата. */
function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const min = Math.floor(diff / 60000);
  if (min < 1) return "только что";
  if (min < 60) return `${min} мин назад`;
  const hours = Math.floor(min / 60);
  if (hours < 24) return `${hours} ч назад`;
  return new Date(iso).toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
}

/**
 * Колокольчик уведомлений в шапке. Показывает счётчик непрочитанных и
 * раскрывает панель со списком. Клик вне панели или Esc закрывают её.
 */
export function NotificationBell() {
  const items = useNotificationStore((s) => s.items);
  const unread = useNotificationStore(selectUnreadCount);
  const markAllRead = useNotificationStore((s) => s.markAllRead);
  const remove = useNotificationStore((s) => s.remove);
  const clear = useNotificationStore((s) => s.clear);

  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  // Закрытие по клику вне панели и по Escape.
  useEffect(() => {
    if (!open) return;
    const onPointer = (e: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("pointerdown", onPointer);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("pointerdown", onPointer);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const toggle = () => {
    setOpen((prev) => {
      const next = !prev;
      if (next && unread > 0) markAllRead();
      return next;
    });
  };

  return (
    <div ref={rootRef} className="relative">
      <button
        onClick={toggle}
        className={cn(
          "relative grid h-10 w-10 place-items-center rounded-xl text-slate-400 transition-colors hover:bg-white/5 hover:text-white",
          open && "bg-white/5 text-white",
        )}
        aria-label={unread > 0 ? `Уведомления: ${unread} новых` : "Уведомления"}
        aria-expanded={open}
      >
        <Bell size={20} />
        {unread > 0 && (
          <span
            className="absolute right-1.5 top-1.5 grid h-4 min-w-4 place-items-center rounded-full px-1 text-[10px] font-bold text-white"
            style={{ backgroundColor: "var(--app-accent)" }}
          >
            {unread > 9 ? "9+" : unread}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-12 z-40 w-80 overflow-hidden rounded-2xl border border-white/10 bg-[#12131f] shadow-2xl">
          <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
            <span className="text-sm font-semibold text-white">Уведомления</span>
            {items.length > 0 && (
              <button
                onClick={clear}
                className="text-xs text-slate-400 transition-colors hover:text-white"
              >
                Очистить
              </button>
            )}
          </div>

          {items.length === 0 ? (
            <div className="grid place-items-center gap-2 px-4 py-10 text-center text-slate-500">
              <Check size={28} className="opacity-40" />
              <p className="text-sm">Пока нет уведомлений</p>
            </div>
          ) : (
            <ul className="max-h-96 overflow-y-auto">
              {items.map((n) => {
                const Icon = KIND_ICON[n.kind];
                return (
                  <li
                    key={n.id}
                    className="group flex items-start gap-3 border-b border-white/5 px-4 py-3 last:border-b-0"
                  >
                    <Icon size={18} className={cn("mt-0.5 shrink-0", KIND_COLOR[n.kind])} />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-medium text-white">{n.title}</div>
                      {n.message && <div className="mt-0.5 text-xs text-slate-400">{n.message}</div>}
                      <div className="mt-1 text-[11px] text-slate-500">{relativeTime(n.createdAt)}</div>
                    </div>
                    <button
                      onClick={() => remove(n.id)}
                      className="shrink-0 text-slate-500 opacity-0 transition-opacity hover:text-white group-hover:opacity-100"
                      aria-label="Убрать уведомление"
                    >
                      <X size={14} />
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
