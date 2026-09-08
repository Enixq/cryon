import { cn } from "../lib/cn";

interface LogoProps {
  size?: number;
  className?: string;
}

/**
 * Иконка-логотип Cryon: неоновая буква «C» (разомкнутое кольцо) с аккуратным
 * трёхполосным эквалайзером внутри — отсылка к звуку и к самому приложению.
 * Векторная, в фиолетово-синей гамме приложения; масштабируется без потерь
 * (шапка, sidebar, экран входа, а также build/appicon.svg для иконки .exe).
 */
export function LogoMark({ size = 36, className }: LogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      className={cn("shrink-0", className)}
      role="img"
      aria-label="Cryon"
    >
      <defs>
        <linearGradient id="cryonBg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
          <stop stopColor="#3b1a6e" />
          <stop offset="1" stopColor="#150c2c" />
        </linearGradient>
        <linearGradient id="cryonStroke" x1="10" y1="8" x2="54" y2="56" gradientUnits="userSpaceOnUse">
          <stop stopColor="#c084fc" />
          <stop offset="0.5" stopColor="#a855f7" />
          <stop offset="1" stopColor="#38bdf8" />
        </linearGradient>
      </defs>

      {/* Скруглённый квадрат-подложка */}
      <rect x="4" y="4" width="56" height="56" rx="16" fill="url(#cryonBg)" stroke="url(#cryonStroke)" strokeWidth="2.5" />

      {/* Буква C как разомкнутое кольцо (разрыв справа) */}
      <path
        d="M44.3 23.4A15 15 0 1 0 44.3 40.6"
        stroke="url(#cryonStroke)"
        strokeWidth="5.5"
        strokeLinecap="round"
        fill="none"
      />

      {/* Трёхполосный эквалайзер внутри «C» */}
      <g fill="#e9d5ff">
        <rect x="24.5" y="27" width="3.4" height="10" rx="1.7" />
        <rect x="30.3" y="23" width="3.4" height="18" rx="1.7" />
        <rect x="36.1" y="25.5" width="3.4" height="13" rx="1.7" />
      </g>
    </svg>
  );
}

/**
 * Полный логотип: иконка + слово «Cryon». Используется в sidebar и на экране
 * входа вместо прежнего текстового логотипа.
 */
export function LogoFull({ className, size = 36 }: LogoProps) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <LogoMark size={size} />
      <span className="text-xl font-bold tracking-tight text-white">Cryon</span>
    </div>
  );
}
