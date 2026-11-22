import { Disc3 } from "lucide-react";
import { Cover } from "./Cover";
import type { AccentColor, SourceId } from "../types";
import { ServiceIcon } from "./ServiceIcon";
import { CollectionArtwork } from "./CollectionArtwork";

export function CollectionCard({
  title,
  subtitle,
  cover,
  accent = "violet",
  onOpen,
  source,
  covers = [],
  catalogKind,
}: {
  title: string;
  subtitle: string;
  cover?: string;
  accent?: AccentColor;
  onOpen: () => void;
  source?: SourceId;
  covers?: string[];
  catalogKind?: string;
}) {
  const artwork = [
    ...new Set([cover, ...covers].filter((url): url is string => Boolean(url))),
  ].slice(0, 4);
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group flex min-w-0 flex-col rounded-2xl border border-white/5 bg-white/[0.025] p-2.5 text-left transition-colors hover:bg-white/[0.07] sm:p-3"
    >
      <div className="relative w-full overflow-hidden rounded-xl">
        {catalogKind ? (
          <CollectionArtwork kind={catalogKind} title={title} />
        ) : artwork.length > 1 ? (
          <div className="grid aspect-square grid-cols-2">
            {Array.from({ length: 4 }, (_, i) => (
              <Cover
                key={i}
                src={artwork[i % artwork.length]}
                accent={accent}
                className="aspect-square"
                rounded="rounded-none"
                alt=""
              />
            ))}
          </div>
        ) : (
          <Cover
            src={artwork[0]}
            alt={title}
            accent={accent}
            className="aspect-square w-full"
            iconSize={0}
          />
        )}
        {!catalogKind && artwork.length === 0 && (
          <div className="pointer-events-none absolute inset-0 overflow-hidden">
            <div className="absolute -right-12 -top-8 h-[85%] w-[85%] rounded-full border-[18px] border-white/10" />
            <div className="absolute -bottom-12 -left-8 h-[80%] w-[80%] rounded-full border-[12px] border-white/10" />
            <div className="absolute left-[18%] top-[18%] rotate-[-12deg] rounded-2xl border border-white/20 bg-black/15 p-5 shadow-2xl">
              <Disc3 size={64} strokeWidth={1.5} className="text-white/90" />
            </div>
            <span className="absolute left-3 top-2 text-[9px] font-bold tracking-[0.3em] text-white/60">
              CRYON COLLECTION
            </span>
          </div>
        )}
        <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent px-3 pb-3 pt-8">
          <span className="line-clamp-2 text-sm font-bold text-white">{title}</span>
        </div>
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
