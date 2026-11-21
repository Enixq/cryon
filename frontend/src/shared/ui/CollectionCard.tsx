import { FolderOpen } from "lucide-react";
import { Cover } from "./Cover";
import type { AccentColor, SourceId } from "../types";
import { ServiceIcon } from "./ServiceIcon";

export function CollectionCard({
  title,
  subtitle,
  cover,
  accent = "violet",
  onOpen,
  source,
}: {
  title: string;
  subtitle: string;
  cover?: string;
  accent?: AccentColor;
  onOpen: () => void;
  source?: SourceId;
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group flex min-w-0 flex-col rounded-2xl border border-white/5 bg-white/[0.025] p-2.5 text-left transition-colors hover:bg-white/[0.07] sm:p-3"
    >
      <div className="relative w-full overflow-hidden rounded-xl">
        <Cover
          src={cover}
          alt={title}
          accent={accent}
          className="aspect-square w-full"
          iconSize={0}
        />
        {!cover && (
          <FolderOpen
            size={44}
            className="pointer-events-none absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 text-white/75"
          />
        )}
        {source && (
          <ServiceIcon id={source} badge size={26} className="absolute bottom-2 right-2" />
        )}
      </div>
      <span className="mt-3 line-clamp-2 text-sm font-semibold text-white sm:text-base">
        {title}
      </span>
      <span className="mt-1 line-clamp-2 text-xs leading-relaxed text-slate-400">{subtitle}</span>
    </button>
  );
}
