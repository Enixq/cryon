// Извлечение доминирующего цвета обложки для подсветки плеера.
//
// Обложка рисуется в маленький offscreen-canvas, откуда усредняются пиксели.
// Так акцентный цвет интерфейса подстраивается под текущий трек. Работает
// только для картинок с одного источника (data-URL или our asset-server),
// иначе canvas «пачкается» cross-origin и getImageData бросает — тогда молча
// откатываемся к дефолтному фиолетовому.

import { useEffect, useState } from "react";

// Дефолтный акцент — тот же фиолетовый, что и в остальном UI.
export const DEFAULT_ACCENT = "#a855f7";

const paletteCache = new Map<string, string>();

/**
 * Возвращает hex доминирующего цвета обложки (или дефолтный акцент, пока
 * цвет не вычислен либо если картинку не удалось прочитать).
 */
export function useCoverPalette(coverUrl: string | undefined): string {
  const [accent, setAccent] = useState<string>(() =>
    coverUrl ? paletteCache.get(coverUrl) ?? DEFAULT_ACCENT : DEFAULT_ACCENT,
  );

  useEffect(() => {
    if (!coverUrl) {
      setAccent(DEFAULT_ACCENT);
      return;
    }
    const cached = paletteCache.get(coverUrl);
    if (cached) {
      setAccent(cached);
      return;
    }

    let cancelled = false;
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      if (cancelled) return;
      const color = dominantColor(img);
      const value = color ?? DEFAULT_ACCENT;
      paletteCache.set(coverUrl, value);
      setAccent(value);
    };
    img.onerror = () => {
      if (!cancelled) setAccent(DEFAULT_ACCENT);
    };
    img.src = coverUrl;

    return () => {
      cancelled = true;
    };
  }, [coverUrl]);

  return accent;
}

// dominantColor усредняет пиксели уменьшенной картинки и слегка поднимает
// насыщенность/яркость, чтобы акцент был живым, а не грязно-серым.
function dominantColor(img: HTMLImageElement): string | null {
  const size = 16;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) return null;
  ctx.drawImage(img, 0, 0, size, size);

  let data: Uint8ClampedArray;
  try {
    data = ctx.getImageData(0, 0, size, size).data;
  } catch {
    // Cross-origin — canvas «испачкан», пиксели прочитать нельзя.
    return null;
  }

  let r = 0;
  let g = 0;
  let b = 0;
  let count = 0;
  for (let i = 0; i < data.length; i += 4) {
    const alpha = data[i + 3];
    if (alpha < 128) continue; // пропускаем прозрачные пиксели
    r += data[i];
    g += data[i + 1];
    b += data[i + 2];
    count++;
  }
  if (count === 0) return null;

  r = Math.round(r / count);
  g = Math.round(g / count);
  b = Math.round(b / count);

  return boost(r, g, b);
}

// boost переводит цвет в HSL, приподнимает насыщенность и держит яркость в
// диапазоне, где текст поверх остаётся читаемым.
function boost(r: number, g: number, b: number): string {
  const [h, s, l] = rgbToHsl(r, g, b);
  const s2 = Math.min(1, Math.max(0.45, s * 1.25));
  const l2 = Math.min(0.62, Math.max(0.42, l));
  const [nr, ng, nb] = hslToRgb(h, s2, l2);
  return "#" + toHex(nr) + toHex(ng) + toHex(nb);
}

function toHex(v: number): string {
  return v.toString(16).padStart(2, "0");
}

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  r /= 255;
  g /= 255;
  b /= 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const l = (max + min) / 2;
  let h = 0;
  let s = 0;
  const d = max - min;
  if (d !== 0) {
    s = d / (1 - Math.abs(2 * l - 1));
    switch (max) {
      case r:
        h = ((g - b) / d) % 6;
        break;
      case g:
        h = (b - r) / d + 2;
        break;
      default:
        h = (r - g) / d + 4;
    }
    h *= 60;
    if (h < 0) h += 360;
  }
  return [h, s, l];
}

function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  let r = 0;
  let g = 0;
  let b = 0;
  if (h < 60) [r, g, b] = [c, x, 0];
  else if (h < 120) [r, g, b] = [x, c, 0];
  else if (h < 180) [r, g, b] = [0, c, x];
  else if (h < 240) [r, g, b] = [0, x, c];
  else if (h < 300) [r, g, b] = [x, 0, c];
  else [r, g, b] = [c, 0, x];
  return [
    Math.round((r + m) * 255),
    Math.round((g + m) * 255),
    Math.round((b + m) * 255),
  ];
}
