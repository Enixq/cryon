import { Music4 } from "lucide-react";
import { useState } from "react";
import type { AccentColor } from "../types";
import { accentStyle } from "./accents";
import { cn } from "../lib/cn";

interface CoverProps {
  accent: AccentColor;
  /** URL обложки. Если не задан или картинка не загрузилась — показывается градиент. */
  src?: string;
  /** Альтернативный текст для картинки (название трека/альбома/плейлиста). */
  alt?: string;
  className?: string;
  rounded?: string;
  iconSize?: number;
  /** Неоновое свечение-рамка в стиле приложения (вместо белой обводки). */
  neon?: boolean;
}

/**
 * Обложка трека/альбома/плейлиста.
 * Если есть src — показывает картинку, иначе (или при ошибке загрузки)
 * рисует градиентную заглушку по акцентному цвету.
 */
export function Cover({
  accent,
  src,
  alt = "",
  className,
  rounded = "rounded-2xl",
  iconSize = 28,
  neon = false,
}: CoverProps) {
  const [failed, setFailed] = useState(false);
  const showImage = Boolean(src) && !failed;

  return (
    <div
      className={cn("relative grid place-items-center overflow-hidden", neon && "neon-frame", rounded, className)}
      style={accentStyle(accent)}
    >
      {showImage ? (
        <img
          src={src}
          alt={alt}
          loading="lazy"
          onError={() => setFailed(true)}
          className="absolute inset-0 h-full w-full object-cover"
        />
      ) : (
        <Music4 size={iconSize} className="text-white/70" />
      )}
    </div>
  );
}
