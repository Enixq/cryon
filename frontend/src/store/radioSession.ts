import { reportDJEvent } from "../shared/api/intelligence";
import type { Track } from "../shared/types";

type RadioSignal = { trackId: string; artist: string; outcome: "skipped" | "completed" };

let signals: RadioSignal[] = [];
let consecutiveSkippedArtist = "";
let consecutiveSkips = 0;
let lastSkippedTrackId = "";
const blockedArtists = new Set<string>();
let completedTrackId = "";
const MAX_SIGNALS = 50;

export function resetRadioSession(): void {
  signals = [];
  consecutiveSkippedArtist = "";
  consecutiveSkips = 0;
  lastSkippedTrackId = "";
  blockedArtists.clear();
  completedTrackId = "";
}

export function recordRadioSignal(track: Track, outcome: RadioSignal["outcome"], playedMs = outcome === "completed" ? track.duration * 1000 : 0, observationOnly = false): void {
  if (!observationOnly) reportDJEvent(track, outcome === "completed" ? "completed" : "skip", playedMs);
  const artist = (track.primaryArtist || track.artist).trim().toLocaleLowerCase();
  completedTrackId = outcome === "completed" ? track.id : "";
  if (outcome === "skipped" && artist) {
    if (track.id !== lastSkippedTrackId) consecutiveSkips = consecutiveSkippedArtist === artist ? consecutiveSkips + 1 : 1;
    consecutiveSkippedArtist = artist;
    lastSkippedTrackId = track.id;
    if (consecutiveSkips >= 2) blockedArtists.add(artist);
  } else {
    consecutiveSkippedArtist = "";
    consecutiveSkips = 0;
    lastSkippedTrackId = "";
  }
  signals = [...signals.filter((signal) => signal.trackId !== track.id), { trackId: track.id, artist, outcome }].slice(-MAX_SIGNALS);
}

export function consumeRadioCompletion(track: Track): boolean {
  if (completedTrackId !== track.id) return false;
  completedTrackId = "";
  return true;
}

export function isRadioArtistBlocked(track: Track): boolean {
  const artist = (track.primaryArtist || track.artist).trim().toLocaleLowerCase();
  return artist !== "" && blockedArtists.has(artist);
}

export function rankRadioTracks(tracks: Track[]): Track[] {
  const skipped = new Set(signals.filter((signal) => signal.outcome === "skipped").map((signal) => signal.trackId));
  const artistScores = new Map<string, number>();
  for (const signal of signals) {
    if (!signal.artist) continue;
    artistScores.set(signal.artist, (artistScores.get(signal.artist) ?? 0) + (signal.outcome === "completed" ? 1 : -1));
  }
  const ranked = tracks
    .filter((track) => !skipped.has(track.id) && !isRadioArtistBlocked(track))
    .map((track, index) => ({ track, index, score: artistScores.get((track.primaryArtist || track.artist).trim().toLocaleLowerCase()) ?? 0 }))
    .sort((left, right) => right.score - left.score || left.index - right.index)
    .map(({ track }) => track);
  const result: Track[] = [];
  const counts = new Map<string, number>();
  const used = new Set<Track>();
  for (let round = 0; round < 2; round++) {
    for (const track of ranked) {
      const artist = (track.primaryArtist || track.artist).trim().toLocaleLowerCase();
      if (used.has(track) || !artist || (counts.get(artist) ?? 0) !== round) continue;
      counts.set(artist, round + 1);
      used.add(track);
      result.push(track);
    }
  }
  // Missing metadata should not make an otherwise playable track disappear.
  return [...result, ...ranked.filter((track) => !(track.primaryArtist || track.artist).trim())];
}
