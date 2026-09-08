// Разбор синхронизированного текста (LRC) и подбор активной строки по времени.
//
// Формат LRC: строки вида «[mm:ss.xx] текст». В одной строке может быть
// несколько таймкодов (повторяющийся припев). Метатеги ([ar:], [ti:], [by:]…)
// пропускаем — нас интересуют только строки с временем.

export interface LyricLine {
  /** Время начала строки в секундах. */
  time: number;
  /** Текст строки (может быть пустым — пауза/проигрыш). */
  text: string;
}

// Таймкод [mm:ss.xx] или [mm:ss]. Группы: минуты, секунды, сотые (опц.).
const TIME_RE = /\[(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?\]/g;

/**
 * Разбирает LRC в отсортированный по времени массив строк. Пустой/невалидный
 * вход даёт пустой массив.
 */
export function parseLrc(lrc: string): LyricLine[] {
  if (!lrc) return [];
  const out: LyricLine[] = [];
  for (const raw of lrc.split(/\r?\n/)) {
    TIME_RE.lastIndex = 0;
    const stamps: number[] = [];
    let m: RegExpExecArray | null;
    let lastEnd = 0;
    while ((m = TIME_RE.exec(raw)) !== null) {
      const min = Number(m[1]);
      const sec = Number(m[2]);
      const frac = m[3] ? Number(m[3]) / Math.pow(10, m[3].length) : 0;
      stamps.push(min * 60 + sec + frac);
      lastEnd = TIME_RE.lastIndex;
    }
    if (stamps.length === 0) continue; // метатег или строка без времени
    const text = raw.slice(lastEnd).trim();
    for (const time of stamps) out.push({ time, text });
  }
  out.sort((a, b) => a.time - b.time);
  return out;
}

/**
 * Индекс активной строки для текущей позиции (сек) — последняя строка, чьё
 * время ≤ position. -1, если позиция раньше первой строки.
 */
export function activeLineIndex(lines: LyricLine[], position: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  let idx = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (lines[mid].time <= position) {
      idx = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return idx;
}
