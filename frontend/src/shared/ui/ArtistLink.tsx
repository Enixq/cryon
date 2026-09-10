import { useNavigate } from "react-router-dom";
import { cn } from "../lib/cn";

// Имена, для которых страница исполнителя бессмысленна: это не конкретный
// артист, а заглушка/сборник. Для них рендерим обычный текст без ссылки.
const NON_NAVIGABLE = new Set(["", "неизвестный исполнитель", "various artists", "va", "unknown artist"]);

interface ArtistLinkProps {
  /** Имя исполнителя (как в track.artist). */
  name: string;
  /** Доп. классы (например, цвет/жирность в конкретном месте). */
  className?: string;
  /**
   * Вызывается ПЕРЕД переходом. Нужно, чтобы закрыть оверлей плеера
   * (полноэкранный мобильный / выдвижную панель), иначе страница исполнителя
   * откроется под ним.
   */
  onNavigate?: () => void;
}

/**
 * Кликабельное имя исполнителя — ведёт на его страницу («аудиотеку»)
 * /artist/:name (маршрут уже есть, данные тянет App.GetArtist по имени).
 *
 * Рендерится инлайновым <span role="link">, а не <button>, специально: так он
 * корректно вписывается в усечение текста (truncate ... white-space:nowrap) и
 * его можно вкладывать внутрь кликабельной строки очереди/мини-плеера — клик по
 * имени гасит всплытие (stopPropagation), поэтому не срабатывает воспроизведение
 * строки, а происходит переход. Для пустых/сборных имён ссылка не создаётся.
 */
export function ArtistLink({ name, className, onNavigate }: ArtistLinkProps) {
  const navigate = useNavigate();
  const trimmed = (name ?? "").trim();

  if (NON_NAVIGABLE.has(trimmed.toLowerCase())) {
    return <span className={className}>{name}</span>;
  }

  const go = () => {
    onNavigate?.();
    navigate(`/artist/${encodeURIComponent(trimmed)}`);
  };

  return (
    <span
      role="link"
      tabIndex={0}
      onClick={(e) => {
        e.stopPropagation();
        e.preventDefault();
        go();
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.stopPropagation();
          e.preventDefault();
          go();
        }
      }}
      className={cn(
        "cursor-pointer outline-none transition-colors hover:text-white hover:underline focus-visible:text-white focus-visible:underline",
        className,
      )}
      title={`Перейти к исполнителю «${trimmed}»`}
    >
      {name}
    </span>
  );
}

export default ArtistLink;
