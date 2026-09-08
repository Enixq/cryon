import { HardDrive } from "lucide-react";
import type { SourceId } from "../types";
import { cn } from "../lib/cn";

interface ServiceIconProps {
  id: SourceId;
  size?: number;
  /** Показывать логотип на фирменном фоне-плашке. */
  badge?: boolean;
  className?: string;
}

// Фирменные цвета фонов для режима badge.
const BRAND_BG: Record<SourceId, string> = {
  spotify: "#1db954",
  soundcloud: "#ff5500",
  youtube: "#ff0000",
  yandex: "#ffdb4d",
  vk: "#0077ff",
  local: "#7c8cff",
};

/**
 * Реальные (стилизованные, лицензионно-нейтральные) логотипы источников
 * музыки вместо текстовых заглушек «sp/sc/yt». Все — простые векторы, чтобы
 * масштабироваться в sidebar, карточках и настройках.
 */
export function ServiceIcon({ id, size = 20, badge = false, className }: ServiceIconProps) {
  const glyph = renderGlyph(id, badge ? Math.round(size * 0.62) : size);
  if (!badge) {
    return <span className={cn("inline-grid place-items-center", className)}>{glyph}</span>;
  }
  return (
    <span
      className={cn("inline-grid place-items-center rounded-md", className)}
      style={{ background: BRAND_BG[id], width: size, height: size }}
    >
      {glyph}
    </span>
  );
}

function renderGlyph(id: SourceId, s: number) {
  switch (id) {
    case "spotify":
      return (
        <svg width={s} height={s} viewBox="0 0 24 24" fill="#1db954" role="img" aria-label="Spotify">
          <circle cx="12" cy="12" r="12" fill="#1db954" />
          <path
            d="M17.6 10.9c-3-1.8-8-2-10.9-1.1a.9.9 0 1 1-.5-1.7c3.3-1 8.8-.8 12.3 1.3a.9.9 0 1 1-.9 1.5zm-.1 2.7c-2.6-1.6-6.5-2-9.5-1.1a.75.75 0 1 1-.4-1.4c3.4-1 7.8-.6 10.7 1.2a.75.75 0 1 1-.8 1.3zm-1.2 2.6c-2-1.3-4.7-1.6-7.7-.9a.6.6 0 1 1-.3-1.2c3.3-.8 6.3-.4 8.6 1a.6.6 0 1 1-.6 1.1z"
            fill="#fff"
          />
        </svg>
      );
    case "soundcloud":
      return (
        <svg width={s} height={s} viewBox="0 0 24 24" role="img" aria-label="SoundCloud">
          <g fill="#ff5500">
            <rect x="2" y="12" width="1.4" height="6" rx="0.7" />
            <rect x="4.4" y="10.5" width="1.4" height="7.5" rx="0.7" />
            <rect x="6.8" y="9" width="1.4" height="9" rx="0.7" />
            <rect x="9.2" y="10" width="1.4" height="8" rx="0.7" />
          </g>
          <path
            d="M12 8.5c.4-2 2.2-3.5 4.4-3.5A4.5 4.5 0 0 1 20.9 9c1.7.1 3.1 1.5 3.1 3.3 0 1.9-1.5 3.4-3.4 3.4H12z"
            fill="#ff5500"
            transform="translate(-1 0)"
          />
        </svg>
      );
    case "youtube":
      return (
        <svg width={s} height={s} viewBox="0 0 24 24" role="img" aria-label="YouTube Music">
          <circle cx="12" cy="12" r="11" fill="#ff0000" />
          <circle cx="12" cy="12" r="6.2" fill="none" stroke="#fff" strokeWidth="1.3" />
          <path d="M10.4 9.2l4 2.8-4 2.8z" fill="#fff" />
        </svg>
      );
    case "yandex":
      return (
        <svg width={s} height={s} viewBox="0 0 24 24" role="img" aria-label="Яндекс Музыка">
          <circle cx="12" cy="12" r="11" fill="#ffdb4d" />
          <path
            d="M13.1 6h-2.2C8.7 6 7.2 7.4 7.2 9.4c0 1.5.7 2.5 2 3.2L7 18h2l2-4.9h.9V18h1.9V6z"
            fill="#000"
          />
        </svg>
      );
    case "vk":
      return (
        <svg width={s} height={s} viewBox="0 0 24 24" role="img" aria-label="VK Музыка">
          <rect width="24" height="24" rx="6" fill="#0077ff" />
          <path
            d="M12.7 16.2c-4.3 0-6.9-3-7-8h2.2c.1 3.6 1.7 5.1 3 5.4V8.2h2v3.1c1.2-.1 2.5-1.5 3-3.1h2c-.3 2-1.7 3.3-2.7 3.9 1 .5 2.6 1.7 3.2 4.1h-2.2c-.5-1.5-1.7-2.7-3.3-2.9v2.9z"
            fill="#fff"
          />
        </svg>
      );
    case "local":
    default:
      return <HardDrive size={s} color="#fff" />;
  }
}
