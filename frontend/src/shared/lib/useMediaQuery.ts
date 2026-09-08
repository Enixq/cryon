import { useEffect, useState } from "react";

/**
 * Подписка на CSS media-query. Возвращает true, пока запрос совпадает.
 *
 * Используется для адаптива каркаса под разные разрешения и
 * DPI-масштабирование Windows: WebView2 отдаёт CSS-пиксели уже с учётом
 * масштаба системы (125/150%), поэтому один и тот же media-query корректно
 * срабатывает и на «узком» ноутбуке, и на 1080p с крупным масштабом.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => {
    if (typeof window === "undefined" || !window.matchMedia) return false;
    return window.matchMedia(query).matches;
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mql = window.matchMedia(query);
    const onChange = () => setMatches(mql.matches);
    onChange();
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, [query]);

  return matches;
}

/**
 * Пороговые ширины каркаса (в CSS-пикселях). Значения подобраны под
 * фиксированную ширину боковой панели и панели «Сейчас играет»: ниже порога
 * контентной колонке не хватает места, поэтому панель схлопывается или
 * скрывается.
 */
export const LAYOUT_BREAKPOINTS = {
  /** Ниже — панель «Сейчас играет» скрывается (её ширина ~340–410px). */
  hideNowPlaying: "(max-width: 1179px)",
  /** Ниже — боковая навигация сворачивается в узкий значковый рельс. */
  collapseSidebar: "(max-width: 959px)",
  /**
   * Телефон: ниже этого порога десктопный каркас (сайдбар + топбар + панель
   * «Сейчас играет» + широкий плеер) заменяется мобильным — нижняя навигация из
   * 5 пунктов, компактный мини-плеер и полноэкранный «Сейчас играет». Порог
   * совпадает с границей Tailwind `sm`, поэтому sm-утилиты внутри страниц
   * включаются ровно на десктопном каркасе.
   */
  phone: "(max-width: 639px)",
} as const;
