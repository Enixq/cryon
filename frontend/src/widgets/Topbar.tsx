import { PanelLeft, Search, Share2, Sparkles } from "lucide-react";
import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { useUiStore } from "../store/uiStore";
import { usePlayerStore } from "../store/playerStore";
import { useMediaQuery, LAYOUT_BREAKPOINTS } from "../shared/lib/useMediaQuery";
import { NotificationBell } from "./NotificationBell";

export function Topbar() {
  const { searchQuery, setSearchQuery } = useUiStore();
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);
  const setNowPlayingOpen = useUiStore((s) => s.setNowPlayingOpen);
  const openShare = useUiStore((s) => s.openShare);
  const hasTrack = usePlayerStore((s) => s.queue.length > 0);
  // Кнопка «Сейчас играет» в топбаре нужна только когда встроенная панель
  // скрыта (узкий экран) — тогда она открывает выдвижную панель.
  const isNarrow = useMediaQuery(LAYOUT_BREAKPOINTS.hideNowPlaying);
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);

  // Ctrl+K фокусирует поиск.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        navigate("/search");
        inputRef.current?.focus();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [navigate]);

  const handleChange = (value: string) => {
    setSearchQuery(value);
    if (value.trim().length > 0) navigate("/search");
  };

  return (
    <header className="flex h-16 shrink-0 items-center gap-3 px-4 sm:gap-4 sm:px-6">
      {/* Свернуть/развернуть боковую панель */}
      <button
        type="button"
        onClick={toggleSidebar}
        className="grid h-10 w-10 shrink-0 place-items-center rounded-xl text-slate-400 transition-colors hover:bg-white/5 hover:text-white"
        aria-label="Свернуть боковую панель"
      >
        <PanelLeft size={20} />
      </button>

      {/* Поиск */}
      <div className="flex h-11 max-w-2xl flex-1 items-center gap-3 rounded-xl bg-white/5 px-4 focus-within:bg-white/8">
        <Search size={18} className="text-slate-400" />
        <input
          ref={inputRef}
          type="text"
          value={searchQuery}
          onChange={(e) => handleChange(e.target.value)}
          onFocus={() => navigate("/search")}
          placeholder="Поиск по трекам, исполнителям, альбомам и плейлистам"
          className="flex-1 bg-transparent text-sm text-white outline-none placeholder:text-slate-500"
          aria-label="Поиск"
        />
        <kbd className="hidden rounded-md bg-white/10 px-2 py-0.5 text-xs text-slate-400 sm:block">Ctrl + K</kbd>
      </div>

      <div className="ml-auto flex items-center gap-3">
        {/* На узком экране встроенная панель «Сейчас играет» скрыта — кнопка
            открывает её как выдвижную поверх контента. */}
        {isNarrow && hasTrack ? (
          <button
            type="button"
            onClick={() => setNowPlayingOpen(true)}
            className="grid h-10 w-10 shrink-0 place-items-center rounded-xl text-slate-400 transition-colors hover:bg-white/5 hover:text-white"
            aria-label="Сейчас играет"
            title="Сейчас играет"
          >
            <Sparkles size={20} />
          </button>
        ) : null}
        <button
          type="button"
          onClick={() => openShare()}
          className="grid h-10 w-10 shrink-0 place-items-center rounded-xl text-slate-400 transition-colors hover:bg-white/5 hover:text-white"
          aria-label="Поделиться треком"
          title="Поделиться / получить трек по ссылке"
        >
          <Share2 size={20} />
        </button>
        <NotificationBell />
      </div>
    </header>
  );
}
