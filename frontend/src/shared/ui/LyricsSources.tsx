import type { LyricsProvider, UseLyricsResult } from "../lib/useLyrics";
export function LyricsSources({
  lyrics,
  hasSource,
}: {
  lyrics: UseLyricsResult;
  hasSource: boolean;
}) {
  const choices: [LyricsProvider, string][] = [
    ["auto", "Авто"],
    ["lrclib", "LRCLIB"],
    ["ovh", "lyrics.ovh"],
  ];
  if (hasSource) choices.push(["source", "Сервис"]);
  return (
    <div className="mb-3 shrink-0">
      <div className="flex flex-wrap gap-1" aria-label="Источник текста">
        {choices.map(([id, label]) => (
          <button
            key={id}
            type="button"
            aria-pressed={lyrics.provider === id}
            onClick={() => lyrics.setProvider(id)}
            className={`rounded-full px-2 py-1.5 text-xs ${lyrics.provider === id ? "bg-[var(--app-accent)] text-white" : "bg-white/5 text-slate-400 hover:text-white"}`}
          >
            {label}
          </button>
        ))}
      </div>
      {lyrics.error && <p role="alert" className="mt-2 text-xs text-slate-400">{lyrics.error}</p>}
      {lyrics.sourceName && (
        <p className="mt-2 text-xs text-slate-500">Текст: {lyrics.sourceName}</p>
      )}
    </div>
  );
}
