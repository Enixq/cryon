import { useNavigate } from "react-router-dom";
import { cn } from "../lib/cn";

const NON_NAVIGABLE = new Set(["", "неизвестный исполнитель", "various artists", "va", "unknown artist"]);

interface ArtistLinkProps {
  name: string;
  className?: string;
  onNavigate?: () => void;
}

function splitArtists(name: string): string[] {
  return name
    .split(/\s*(?:,|&|\bfeat\.?\b|\bft\.?\b)\s*/i)
    .map((artist) => artist.trim())
    .filter((artist, index, artists) => artist && artists.indexOf(artist) === index);
}

export function ArtistLink({ name, className, onNavigate }: ArtistLinkProps) {
  const navigate = useNavigate();
  const artists = splitArtists(name);

  if (artists.length === 0 || artists.every((artist) => NON_NAVIGABLE.has(artist.toLowerCase()))) {
    return <span className={className}>{name}</span>;
  }

  const go = (artist: string) => {
    onNavigate?.();
    navigate(`/artist/${encodeURIComponent(artist)}`);
  };

  return (
    <span className={cn("inline", className)}>
      {artists.map((artist, index) => {
        const disabled = NON_NAVIGABLE.has(artist.toLowerCase());
        return (
          <span key={`${artist}-${index}`}>
            {index > 0 ? <span className="text-slate-500">, </span> : null}
            {disabled ? (
              <span>{artist}</span>
            ) : (
              <span
                role="link"
                tabIndex={0}
                onClick={(event) => { event.stopPropagation(); event.preventDefault(); go(artist); }}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.stopPropagation();
                    event.preventDefault();
                    go(artist);
                  }
                }}
                className="cursor-pointer outline-none transition-colors hover:text-white hover:underline focus-visible:text-white focus-visible:underline"
                title={artist}
              >
                {artist}
              </span>
            )}
          </span>
        );
      })}
    </span>
  );
}

export default ArtistLink;
