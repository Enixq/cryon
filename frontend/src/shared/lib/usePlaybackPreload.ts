import { useEffect } from "react";
import { usePlayerStore } from "../../store/playerStore";
import {
  getAudioStreamInfo,
  getPlaybackUrlForHtml5,
  resolvePlayableTrack,
  isWailsRuntime,
} from "../api/client";
import { hasNativeAndroidQueue } from "./nativeAndroidPlayer";

/** Resolve and warm the next stream while the current track is playing. */
export function usePlaybackPreload(): void {
  const next = usePlayerStore((s) => s.queue[s.currentIndex + 1]);
  const local = usePlayerStore((s) => s.playbackOnThisDevice);
  const playing = usePlayerStore((s) => s.isPlaying);
  useEffect(() => {
    if (!next || !local || !playing || !isWailsRuntime() || hasNativeAndroidQueue()) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), 120_000);
    void (async () => {
      const playable = await resolvePlayableTrack(next);
      if (controller.signal.aborted) return;
      await getAudioStreamInfo(playable.id).catch(() => null);
      const url = await getPlaybackUrlForHtml5(playable.id);
      if (!url || controller.signal.aborted) return;
      const response = await fetch(url, {
        headers: { Range: "bytes=0-0" },
        signal: controller.signal,
      });
      await response.body?.cancel();
    })()
      .catch(() => {})
      .finally(() => window.clearTimeout(timer));
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [next, local, playing]);
}
