import type { Track } from "../shared/types";

type RadioSignal = { trackId: string; artist: string; outcome: "skipped" | "completed" };

let signals: RadioSignal[] = [];
const MAX_SIGNALS = 50;

export function resetRadioSession(): void {
  signals = [];
}

export function recordRadioSignal(track: Track, outcome: RadioSignal["outcome"]): void {
  const artist = track.artist.trim().toLocaleLowerCase();
  signals = [...signals.filter((signal) => signal.trackId !== track.id), { trackId: track.id, artist, outcome }].slice(-MAX_SIGNALS);
}

export function rankRadioTracks(tracks: Track[]): Track[] {
  const skipped = new Set(signals.filter((signal) => signal.outcome === "skipped").map((signal) => signal.trackId));
  const artistScores = new Map<string, number>();
  for (const signal of signals) {
    if (!signal.artist) continue;
    artistScores.set(signal.artist, (artistScores.get(signal.artist) ?? 0) + (signal.outcome === "completed" ? 1 : -1));
  }
  return tracks
    .filter((track) => !skipped.has(track.id))
    .map((track, index) => ({ track, index, score: artistScores.get(track.artist.trim().toLocaleLowerCase()) ?? 0 }))
    .sort((left, right) => right.score - left.score || left.index - right.index)
    .map(({ track }) => track);
}
