import { cn } from "../lib/cn";
interface LogoProps {
  size?: number;
  className?: string;
}
/** A monochrome C, waveform and play mark; cover colour comes from the shell. */
export function LogoMark({ size = 36, className }: LogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 512 512"
      fill="none"
      className={cn("cryon-logo shrink-0", className)}
      role="img"
      aria-label="Cryon"
    >
      <path
        d="M420 139a184 184 0 1 0 0 234"
        stroke="currentColor"
        strokeWidth="52"
        strokeLinecap="round"
      />
      <path
        d="M52 256h18m14-14v28m24-59v90m25-121v153m26-199v245m27-121v98m27-56v36m18-18h24"
        stroke="currentColor"
        strokeWidth="13"
        strokeLinecap="round"
      />
      <path
        d="M281 176c0-9 10-14 18-9l125 78c9 6 9 16 0 22l-125 78c-8 5-18 0-18-9z"
        fill="currentColor"
      />
    </svg>
  );
}
export function LogoFull({ size = 36, className }: LogoProps) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <LogoMark size={size} />
      <span className="text-xl font-bold tracking-tight text-white">Cryon</span>
    </div>
  );
}
