// Observed forward playback only. Seeks and long gaps are not listening time.
const observations = new Map<string, { position: number; at: number; heard: number }>();
export function observeListening(
  id: string,
  position: number,
  playing: boolean,
  now = performance.now(),
): void {
  if (!Number.isFinite(position)) return;
  const previous = observations.get(id);
  let heard = previous?.heard ?? 0;
  if (previous && playing) {
    const delta = position - previous.position;
    const elapsed = (now - previous.at) / 1000;
    if (delta > 0 && elapsed >= 0 && elapsed <= 5 && delta <= Math.min(5, elapsed * 1.5 + 0.2))
      heard += delta * 1000;
  }
  observations.set(id, { position, at: now, heard });
  if (observations.size > 100) observations.delete(observations.keys().next().value!);
}
export function consumeListening(id: string): number {
  const heard = observations.get(id)?.heard ?? 0;
  observations.delete(id);
  return Math.round(heard);
}
