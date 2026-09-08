// Окно «Поделиться треком»: сверху — ссылка на выбранный трек для отправки
// другу, снизу — поле, куда друг вставляет полученную ссылку, чтобы добавить
// трек себе. Без сервера и регистрации (см. shared/lib/trackShare.ts).

import { useEffect, useMemo, useState } from "react";
import { Check, Copy, ExternalLink, Play, Heart, Share2, X } from "lucide-react";
import { useUiStore } from "../store/uiStore";
import { usePlayerStore } from "../store/playerStore";
import { Cover } from "../shared/ui/Cover";
import { sourceName } from "../shared/sources";
import { encodeTrackShare, decodeTrackShare } from "../shared/lib/trackShare";
import { addFavorite, openExternal } from "../shared/api/client";
import { notify } from "../store/notificationStore";
import type { Track } from "../shared/types";
import { cn } from "../shared/lib/cn";

export function ShareTrackModal() {
  const open = useUiStore((s) => s.shareOpen);
  const shareTrack = useUiStore((s) => s.shareTrack);
  const close = useUiStore((s) => s.closeShare);

  const playTrack = usePlayerStore((s) => s.playTrack);

  const [copied, setCopied] = useState(false);
  const [received, setReceived] = useState("");
  const [decoded, setDecoded] = useState<Track | null>(null);
  const [decodeError, setDecodeError] = useState(false);

  const shareLink = useMemo(() => (shareTrack ? encodeTrackShare(shareTrack) : ""), [shareTrack]);

  // Сбрасываем локальное состояние при каждом открытии/смене трека.
  useEffect(() => {
    if (open) {
      setCopied(false);
      setReceived("");
      setDecoded(null);
      setDecodeError(false);
    }
  }, [open, shareTrack]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, close]);

  if (!open) return null;

  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(shareLink);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      notify({ kind: "warning", title: "Не удалось скопировать", message: "Скопируйте ссылку вручную из поля." });
    }
  };

  const handleReceivedChange = (value: string) => {
    setReceived(value);
    const track = decodeTrackShare(value);
    setDecoded(track);
    setDecodeError(value.trim().length > 0 && !track);
  };

  const playReceived = () => {
    if (!decoded) return;
    playTrack(decoded, [decoded]);
    notify({ kind: "success", title: "Трек добавлен", message: `${decoded.artist} — ${decoded.title}` });
    close();
  };

  const favoriteReceived = async () => {
    if (!decoded) return;
    await addFavorite(decoded).catch(() => {});
    notify({ kind: "success", title: "В избранном", message: `${decoded.artist} — ${decoded.title}` });
  };

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm"
      onClick={close}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="share-title"
        className="w-full max-w-md rounded-3xl border border-white/15 bg-[#10121e] p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-[color-mix(in_srgb,var(--app-accent)_25%,transparent)] text-[var(--app-accent)]">
              <Share2 size={20} />
            </div>
            <div>
              <h2 id="share-title" className="text-lg font-bold text-white">
                Поделиться треком
              </h2>
              <p className="text-xs text-slate-400">Ссылкой — без регистрации</p>
            </div>
          </div>
          <button
            aria-label="Закрыть"
            onClick={close}
            className="text-slate-400 transition-colors hover:text-white"
          >
            <X size={20} />
          </button>
        </div>

        {/* Отправить: ссылка на выбранный трек */}
        {shareTrack && (
          <div className="mt-5">
            <div className="flex items-center gap-3 rounded-2xl bg-white/[0.04] p-3">
              <Cover accent={shareTrack.accent} src={shareTrack.coverUrl} alt={shareTrack.title} className="h-12 w-12 shrink-0" iconSize={18} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-semibold text-white">{shareTrack.title}</div>
                <div className="truncate text-xs text-slate-400">
                  {shareTrack.artist} · {sourceName(shareTrack.source)}
                </div>
              </div>
            </div>

            <div className="mt-3 flex items-center gap-2">
              <input
                readOnly
                value={shareLink}
                onFocus={(e) => e.currentTarget.select()}
                className="min-w-0 flex-1 truncate rounded-xl border border-white/8 bg-black/20 px-3 py-2.5 text-xs text-slate-300 outline-none"
                aria-label="Ссылка на трек"
              />
              <button
                onClick={copyLink}
                className={cn(
                  "flex shrink-0 items-center gap-1.5 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors",
                  copied ? "bg-emerald-500 text-white" : "bg-[var(--app-accent)] text-slate-950 hover:opacity-90",
                )}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
                {copied ? "Готово" : "Копировать"}
              </button>
            </div>

            {shareTrack.externalUrl && (
              <button
                onClick={() => void openExternal(shareTrack.externalUrl!).catch(() => {})}
                className="mt-2 flex items-center gap-1.5 text-xs text-slate-400 transition-colors hover:text-white"
              >
                <ExternalLink size={13} /> Открыть в сервисе
              </button>
            )}

            <p className="mt-3 text-xs text-slate-500">
              Отправьте ссылку другу в любом мессенджере — он вставит её ниже в своём Cryon.
            </p>
          </div>
        )}

        {/* Получить: вставить ссылку от друга */}
        <div className={cn(shareTrack && "mt-5 border-t border-white/8 pt-5")}>
          <label className="text-sm font-medium text-white">Получить трек по ссылке</label>
          <textarea
            value={received}
            onChange={(e) => handleReceivedChange(e.target.value)}
            placeholder="Вставьте ссылку cryon://track/… от друга"
            rows={2}
            className="mt-2 w-full resize-none rounded-xl border border-white/8 bg-black/20 px-3 py-2.5 text-xs text-slate-200 outline-none placeholder:text-slate-500 focus:border-white/20"
            spellCheck={false}
          />
          {decodeError && (
            <p className="mt-1 text-xs text-rose-300">Не похоже на ссылку Cryon. Проверьте, что скопировали её целиком.</p>
          )}
          {decoded && (
            <div className="mt-3 rounded-2xl bg-white/[0.04] p-3">
              <div className="flex items-center gap-3">
                <Cover accent={decoded.accent} src={decoded.coverUrl} alt={decoded.title} className="h-12 w-12 shrink-0" iconSize={18} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-semibold text-white">{decoded.title}</div>
                  <div className="truncate text-xs text-slate-400">
                    {decoded.artist} · {sourceName(decoded.source)}
                  </div>
                </div>
              </div>
              <div className="mt-3 flex gap-2">
                <button
                  onClick={playReceived}
                  className="flex flex-1 items-center justify-center gap-1.5 rounded-xl bg-[var(--app-accent)] px-3 py-2 text-sm font-semibold text-slate-950 transition-opacity hover:opacity-90"
                >
                  <Play size={15} fill="currentColor" /> Слушать
                </button>
                <button
                  onClick={favoriteReceived}
                  className="flex items-center justify-center gap-1.5 rounded-xl bg-white/10 px-3 py-2 text-sm font-medium text-slate-200 transition-colors hover:bg-white/15"
                >
                  <Heart size={15} /> В избранное
                </button>
              </div>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
