import {
  Download,
  Heart,
  ListPlus,
  ListStart,
  MoreHorizontal,
  Radio,
  Share2,
  ThumbsDown,
} from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { usePlayerStore } from "../../store/playerStore";
import { useUiStore } from "../../store/uiStore";
import type { Track } from "../types";
import { AddToPlaylistButton } from "./AddToPlaylistButton";
import { downloadTrack } from "../lib/downloadTrack";
import { notify } from "../../store/notificationStore";
import { cn } from "../lib/cn";
import { useFavoriteIds } from "../lib/useFavorites";
import { useRecoFeedback } from "../lib/useRecoFeedback";

interface Props {
  track: Track;
  playerControl?: boolean;
}

export function TrackQuickActions({ track, playerControl = false }: Props) {
  const [open, setOpen] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const favoriteIds = useFavoriteIds();
  const isFavorite = favoriteIds.has(track.id);
  const addToQueue = usePlayerStore((state) => state.addToQueue);
  const playNext = usePlayerStore((state) => state.playNext);
  const playTrack = usePlayerStore((state) => state.playTrack);
  const openRadioPreview = useUiStore((state) => state.openRadioPreview);
  const openSourcePicker = useUiStore((state) => state.openSourcePicker);
  const toggleLikeWithTrack = usePlayerStore((state) => state.toggleLikeWithTrack);
  const openShare = useUiStore((state) => state.openShare);
  const feedback = useRecoFeedback();
  const disliked = feedback.scoreFor(track) < 0;

  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const trigger = triggerRef.current?.getBoundingClientRect();
      const menu = menuRef.current?.getBoundingClientRect();
      if (!trigger || !menu) return;
      const viewportMargin = 8;
      const menuWidth = Math.min(menu.width, window.innerWidth - viewportMargin * 2);
      const below = window.innerHeight - trigger.bottom;
      const preferredTop =
        below >= menu.height + 6 ? trigger.bottom + 6 : trigger.top - menu.height - 6;
      const top = Math.max(
        viewportMargin,
        Math.min(preferredTop, window.innerHeight - menu.height - viewportMargin),
      );
      const left = Math.max(
        viewportMargin,
        Math.min(trigger.right - menuWidth, window.innerWidth - menuWidth - viewportMargin),
      );
      setPosition({ top, left });
    };
    const dismiss = (event: PointerEvent) => {
      if ((event.target as Element).closest("[data-track-playlist-dialog]")) return;
      if (
        !triggerRef.current?.contains(event.target as Node) &&
        !menuRef.current?.contains(event.target as Node)
      )
        setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        triggerRef.current?.focus();
      }
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  useLayoutEffect(() => {
    const row = triggerRef.current?.closest("[data-track-row]");
    if (!row) return;
    const show = (event: Event) => {
      event.preventDefault();
      setOpen(true);
    };
    row.addEventListener("contextmenu", show);
    return () => row.removeEventListener("contextmenu", show);
  }, []);

  return (
    <div className="relative flex items-center justify-center gap-0.5">
      {!playerControl && (
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation();
            addToQueue(track);
          }}
          className="hidden h-9 w-9 place-items-center rounded-full text-lg text-slate-400 transition-colors hover:bg-[color-mix(in_srgb,var(--app-accent)_18%,transparent)] hover:text-white active:scale-95 sm:grid"
          aria-label="Добавить в очередь"
        >
          +
        </button>
      )}
      <button
        ref={triggerRef}
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          setOpen((value) => !value);
        }}
        className={cn(
          "shrink-0 rounded-full text-slate-400 transition-colors hover:bg-white/10 hover:text-white active:scale-95",
          playerControl ? "grid h-11 w-11 place-items-center" : "grid h-9 w-9 place-items-center",
        )}
        aria-label="Другие действия"
        aria-expanded={open}
        data-track-actions-trigger
      >
        <MoreHorizontal size={playerControl ? 20 : 17} />
      </button>
      {open &&
        createPortal(
          <div
            ref={menuRef}
            role="menu"
            style={{ top: position.top, left: position.left }}
            className="fixed z-[100] max-h-[calc(100vh-16px)] w-56 max-w-[calc(100vw-16px)] overflow-visible rounded-xl border border-white/10 bg-[#171722] p-2 shadow-2xl"
            onClick={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              onClick={() => {
                openRadioPreview(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <Radio size={15} /> Радио по этому треку
            </button>
            <button
              type="button"
              onClick={() => {
                openSourcePicker(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <ListStart size={15} /> Сменить источник трека
            </button>
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                playNext(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <ListStart size={15} />
              Играть дальше
            </button>
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                addToQueue(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <ListPlus size={15} />
              Добавить в очередь
            </button>
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                playTrack(track, [track]);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <ListPlus size={15} />
              Играть сейчас
            </button>
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                toggleLikeWithTrack({ ...track, liked: isFavorite });
                setOpen(false);
              }}
              className={cn(
                "flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm hover:bg-white/8",
                isFavorite ? "text-[#a855f7]" : "text-slate-200",
              )}
            >
              <Heart size={15} fill={isFavorite ? "currentColor" : "none"} />
              {isFavorite ? "Убрать из избранного" : "В избранное"}
            </button>
            <button
              type="button"
              disabled={feedback.saving}
              onClick={(event) => {
                event.stopPropagation();
                feedback.rate(track, -1);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8 disabled:opacity-50"
            >
              <ThumbsDown size={15} />
              {disliked ? "Вернуть в рекомендации" : "Не рекомендовать трек"}
            </button>
            <button
              type="button"
              disabled={feedback.saving}
              onClick={() => {
                feedback.toggleArtist(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8 disabled:opacity-50"
            >
              <ThumbsDown size={15} />
              {feedback.artistBlocked(track)
                ? "Разрешить исполнителя"
                : "Не рекомендовать исполнителя"}
            </button>
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                openShare(track);
                setOpen(false);
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8"
            >
              <Share2 size={15} />
              Поделиться
            </button>
            <button
              type="button"
              disabled={downloading}
              onClick={() => {
                setDownloading(true);
                void downloadTrack(track)
                  .then((saved) => {
                    if (saved)
                      notify({
                        kind: "success",
                        title: "Трек сохранён",
                        message: "Трек доступен без сети в разделе «Локальная музыка → Скачано».",
                      });
                  })
                  .catch((error) =>
                    notify({ kind: "error", title: "Не удалось скачать", message: error.message }),
                  )
                  .finally(() => setDownloading(false));
              }}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-200 hover:bg-white/8 disabled:opacity-50"
            >
              <Download size={15} />
              {downloading ? "Скачивание…" : "Скачать аудиофайл"}
            </button>
            <div className="mt-1 flex items-center gap-2 border-t border-white/8 pt-2">
              <AddToPlaylistButton track={track} opensUp alwaysVisible showLabel />
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
