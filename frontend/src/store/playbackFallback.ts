import type { Track } from "../shared/types";

const MAX_ATTEMPTS = 8;

export class PlaybackFallbackSession {
  anchor: Track | null = null;
  attempted = new Set<string>();
  resumePosition = 0;
  pending: { trackId: string; from: Track; position: number; reason: string; searchMs: number } | null = null;

  fail(track: Track, position: number): boolean {
    if (this.attempted.has(track.id) || this.attempted.size >= MAX_ATTEMPTS) return false;
    this.anchor ??= track;
    this.attempted.add(track.id);
    if (Number.isFinite(position) && position > 0) this.resumePosition = position;
    this.pending = null;
    return true;
  }

  select(track: Track, from: Track, reason: string, searchMs: number) {
    this.pending = { trackId: track.id, from, position: this.resumePosition, reason, searchMs };
  }

  started(track: Track) {
    if (this.pending?.trackId !== track.id) return null;
    const pending = this.pending;
    this.pending = null;
    this.resumePosition = 0;
    return pending;
  }

  reset() {
    this.anchor = null;
    this.attempted.clear();
    this.resumePosition = 0;
    this.pending = null;
  }
}
