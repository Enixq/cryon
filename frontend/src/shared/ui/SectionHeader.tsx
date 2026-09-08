interface SectionHeaderProps {
  title: string;
  /** Необязательный подзаголовок под названием секции. */
  subtitle?: string;
  action?: string;
  onAction?: () => void;
}

export function SectionHeader({ title, subtitle, action = "Показать все", onAction }: SectionHeaderProps) {
  return (
    <div className="mb-4 flex items-center justify-between">
      <div>
        <h2 className="text-xl font-bold text-white">{title}</h2>
        {subtitle && <p className="text-sm text-slate-400">{subtitle}</p>}
      </div>
      <div className="flex items-center gap-2">
        {onAction && (
          <button
            onClick={onAction}
            className="text-sm text-slate-400 transition-colors hover:text-white"
          >
            {action}
          </button>
        )}
      </div>
    </div>
  );
}
