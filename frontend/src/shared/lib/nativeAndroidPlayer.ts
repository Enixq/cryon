import type { Track } from "../types";

export type NativePlaybackState = {
  trackId: string;
  state: "idle" | "buffering" | "ready" | "ended" | "error";
  playing: boolean;
  positionMs: number;
  durationMs: number;
  error?: string;
};

type NativeAndroidBridge = {
  savePlaybackReport?: (report: string) => void;
  installUpdate?: () => void;
  nativePlay?: (streamUrl: string, trackId: string, title: string, artist: string, artworkUrl: string, durationMs: number, autoPlay: boolean, startPositionMs: number, mimeType: string) => void;
  nativePause?: () => void;
  nativeResume?: () => void;
  nativeStop?: () => void;
  nativeSeek?: (positionMs: number) => void;
  nativeSetVolume?: (volume: number) => void;
};

declare global {
  interface Window {
    CryonAndroid?: NativeAndroidBridge;
  }
}

const eventName = "cryon:native-playback";

function bridge(): NativeAndroidBridge | undefined {
  return typeof window === "undefined" ? undefined : window.CryonAndroid;
}

export function hasNativeAndroidPlayer(): boolean {
  return typeof bridge()?.nativePlay === "function";
}

export function playNativeAndroidTrack(track: Track, streamUrl: string, autoPlay: boolean, startPositionSeconds = 0, mimeType = ""): void {
  bridge()?.nativePlay?.(
    streamUrl,
    track.id,
    track.title,
    track.artist,
    track.coverUrl || "",
    Math.max(0, Math.round((track.duration || 0) * 1000)),
    autoPlay,
    Math.max(0, Math.round(startPositionSeconds * 1000)),
    mimeType,
  );
}

export function pauseNativeAndroidPlayer(): void { bridge()?.nativePause?.(); }
export function resumeNativeAndroidPlayer(): void { bridge()?.nativeResume?.(); }
export function stopNativeAndroidPlayer(): void { bridge()?.nativeStop?.(); }
export function seekNativeAndroidPlayer(seconds: number): void { bridge()?.nativeSeek?.(Math.max(0, Math.round(seconds * 1000))); }
export function setNativeAndroidPlayerVolume(volume: number): void { bridge()?.nativeSetVolume?.(Math.max(0, Math.min(100, Math.round(volume)))); }

export function subscribeNativeAndroidPlayback(listener: (state: NativePlaybackState) => void): () => void {
  if (typeof window === "undefined") return () => {};
  const handler = (event: Event) => listener((event as CustomEvent<NativePlaybackState>).detail);
  window.addEventListener(eventName, handler);
  return () => window.removeEventListener(eventName, handler);
}
