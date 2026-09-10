// Единая «шапка» экрана: заголовок, необязательный подзаголовок, значок в
// акцентной плитке и блок действий справа. Раньше каждая страница верстала
// заголовок по-своему (то text-2xl, то text-3xl, где-то со значком, где-то с
// захардкоженным фиолетовым), из-за чего вкладки выглядели разнородно. Теперь
// размер и отступы одинаковы везде, а акцент берётся из текущей обложки
// (var(--app-accent)), как и во всём приложении.

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  /** Короткое пояснение под заголовком. */
  subtitle?: string;
  /** Значок слева от заголовка (в акцентной плитке). */
  icon?: LucideIcon;
  /** Кнопки/действия справа. */
  actions?: ReactNode;
}

export function PageHeader({ title, subtitle, icon: Icon, actions }: PageHeaderProps) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex items-center gap-3">
        {Icon && (
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl bg-[color-mix(in_srgb,var(--app-accent)_22%,transparent)] text-[var(--app-accent)]">
            <Icon size={22} />
          </div>
        )}
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold text-white sm:text-3xl">{title}</h1>
          {subtitle && <p className="mt-0.5 text-sm text-slate-400">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
