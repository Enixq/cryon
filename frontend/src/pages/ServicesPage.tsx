import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronRight, LayoutGrid } from "lucide-react";
import { PageHeader } from "../shared/ui/PageHeader";
import { ServiceIcon } from "../shared/ui/ServiceIcon";
import { SOURCES } from "../shared/sources";
import { listSourceStatus, onSourceStatusChanged } from "../shared/api/client";
import type { SourceLevel } from "../shared/api/client";
import type { SourceId } from "../shared/types";
import { cn } from "../shared/lib/cn";

// Цвет точки и подпись статуса — те же градации, что в боковой панели.
const STATUS_META: Record<SourceLevel, { color: string; label: string }> = {
  ok: { color: "bg-emerald-400", label: "Подключено — играет напрямую" },
  limited: { color: "bg-amber-400", label: "Ограниченно — поиск / внешние ссылки" },
  off: { color: "bg-rose-500", label: "Не подключено" },
};

// Порядок карточек. VK исключён (нефункционален, убран из поиска), Spotify
// оставлен — он рабочий для импорта лайков и «Радара новинок».
const order: SourceId[] = ["youtube", "soundcloud", "yandex", "spotify", "local"];

/**
 * Экран «Сервисы» — отдельный пункт нижней навигации на телефоне. Показывает
 * источники музыки и их состояние; тап по карточке ведёт в настройки для
 * подключения/ввода ключей. Статус берётся из backend (listSourceStatus) и
 * обновляется по событию source:status-changed.
 */
export function ServicesPage() {
  const navigate = useNavigate();
  const [levels, setLevels] = useState<Record<string, SourceLevel>>({});

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
    <div className="flex flex-col gap-5 py-2">
      <PageHeader title="Сервисы" subtitle="Источники музыки и их состояние" icon={LayoutGrid} />

      <div className="flex flex-col gap-2">
        {order.map((id) => {
          const meta = SOURCES[id];
          const level = levels[id] ?? "off";
          const status = STATUS_META[level];
          return (
            <button
              key={id}
              type="button"
              onClick={() => navigate("/settings")}
              className="neon-card flex items-center gap-3 rounded-2xl p-3 text-left"
              title={`Настроить ${meta.name}`}
            >
              <ServiceIcon id={id} size={44} badge className="shrink-0" />
              <div className="min-w-0 flex-1">
                <div className="truncate text-[15px] font-semibold text-white">{meta.name}</div>
                <div className="mt-0.5 flex items-center gap-1.5 text-xs text-slate-400">
                  <span className={cn("h-2 w-2 shrink-0 rounded-full", status.color)} aria-hidden />
                  <span className="truncate">{status.label}</span>
                </div>
              </div>
              <ChevronRight size={18} className="shrink-0 text-slate-500" />
            </button>
          );
        })}
      </div>

      <p className="px-1 text-xs leading-relaxed text-slate-500">
        Рабочие источники поиска: YouTube Music, SoundCloud, Яндекс Музыка и локальная
        библиотека. Spotify используется для импорта избранного и «Радара новинок».
      </p>
    </div>
  );
}
