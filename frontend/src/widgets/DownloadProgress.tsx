import { X } from "lucide-react";
import { useDownloadStore } from "../store/downloadStore";
export function DownloadProgress() {
  const items = useDownloadStore((s) => s.items);
  if (!items.length) return null;
  return (
    <div
      className="fixed bottom-28 right-3 z-[160] flex w-[min(340px,calc(100vw-24px))] flex-col gap-2 sm:bottom-24"
      aria-live="polite"
    >
      {items.slice(-3).map((item) => (
        <div key={item.id} className="rounded-xl border border-white/10 bg-[#171722] p-3 shadow-xl">
          <div className="flex items-start gap-2">
            <span className="min-w-0 flex-1 truncate text-sm text-white">{item.title}</span>
            <button
              aria-label="Скрыть уведомление загрузки"
              onClick={() => useDownloadStore.getState().dismiss(item.id)}
              className="text-slate-400"
            >
              <X size={16} />
            </button>
          </div>
          <p
            className={`mt-1 text-xs ${item.state === "error" ? "text-rose-300" : "text-slate-400"}`}
          >
            {item.message}
          </p>
          {item.state === "loading" && (
            <div
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(item.progress * 100)}
              className="mt-2 h-1 overflow-hidden rounded bg-white/10"
            >
              <div
                className="h-full bg-[var(--app-accent)] transition-[width]"
                style={{ width: `${Math.max(2, item.progress * 100)}%` }}
              />
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
