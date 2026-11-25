import type { Track } from "../types";

export type NativePlaybackState = {
  trackId: string;
  state: "idle" | "buffering" | "ready" | "ended" | "error";
  playing: boolean;
  positionMs: number;
  durationMs: number;
  error?: string;
  queue?: Track[];
  currentIndex?: number;
  autoPlay?: boolean;
  radio?: boolean;
  radioSeed?: Track | null;
  playbackOnThisDevice?: boolean;
  commandId?: number;
};

type NativeAndroidBridge = {
	openVKLogin?: (sessionId: string) => void;
	openVKFreshLogin?: (sessionId: string) => void;
	openVKWebsite?: () => void;
  nativeDownload?: (url: string) => string;
  nativeDeleteDownload?: (key: string) => boolean;
  nativeDownloadStatus?: (ticket: string) => string;
  savePlaybackReport?: (report: string) => void;
  installUpdate?: () => void;
  nativePlay?: (
    streamUrl: string,
    trackId: string,
    title: string,
    artist: string,
    artworkUrl: string,
    durationMs: number,
    autoPlay: boolean,
    startPositionMs: number,
    mimeType: string,
    commandId?: number,
  ) => void;
  nativePause?: (commandId?: number) => void;
  nativeResume?: (commandId?: number) => void;
  nativeStop?: (commandId?: number) => void;
  nativeSeek?: (positionMs: number) => void;
  nativeSetVolume?: (volume: number) => void;
  nativeSetQueue?: (json: string) => void;
  nativeGetState?: () => string;
  nativeConfigureSync?: (json: string) => void;
};

declare global {
  interface Window {
    CryonAndroid?: NativeAndroidBridge;
  }
}

const eventName = "cryon:native-playback";
let lastCommandId = 0;
function nextCommandId(): number {
  return (lastCommandId = Math.max(Date.now() * 1000, lastCommandId + 1));
}

function bridge(): NativeAndroidBridge | undefined {
  return typeof window === "undefined" ? undefined : window.CryonAndroid;
}

export function hasNativeAndroidPlayer(): boolean {
  return typeof bridge()?.nativePlay === "function";
}

export function hasNativeAndroidQueue(): boolean {
  return typeof bridge()?.nativeSetQueue === "function";
}
export function updateNativeAndroidQueue(state: object): void {
  bridge()?.nativeSetQueue?.(JSON.stringify(state));
}
export function configureNativeAndroidSync(state: object): void {
  bridge()?.nativeConfigureSync?.(JSON.stringify(state));
}

export function playNativeAndroidTrack(
  track: Track,
  streamUrl: string,
  autoPlay: boolean,
  startPositionSeconds = 0,
  mimeType = "",
): number {
  const commandId = nextCommandId();
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
    commandId,
  );
  return commandId;
}

export function pauseNativeAndroidPlayer(): number {
  const id = nextCommandId();
  bridge()?.nativePause?.(id);
  return id;
}
export function resumeNativeAndroidPlayer(): number {
  const id = nextCommandId();
  bridge()?.nativeResume?.(id);
  return id;
}
export function stopNativeAndroidPlayer(): number {
  const id = nextCommandId();
  bridge()?.nativeStop?.(id);
  return id;
}
export async function stopNativeAndroidPlayerAndWait(): Promise<void> {
  stopNativeAndroidPlayer();
  if (!bridge()?.nativeGetState) return;
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    const saved = bridge()?.nativeGetState?.();
    if (!saved) return;
    const state = JSON.parse(saved) as NativePlaybackState;
    if (!state.trackId && !state.playing) return;
    await new Promise((resolve) => window.setTimeout(resolve, 25));
  }
  throw new Error("Android player did not acknowledge stop");
}
export function seekNativeAndroidPlayer(seconds: number): void {
  bridge()?.nativeSeek?.(Math.max(0, Math.round(seconds * 1000)));
}
export function setNativeAndroidPlayerVolume(volume: number): void {
  bridge()?.nativeSetVolume?.(Math.max(0, Math.min(100, Math.round(volume))));
}

export function subscribeNativeAndroidPlayback(
  listener: (state: NativePlaybackState) => void,
): () => void {
  if (typeof window === "undefined") return () => {};
  const handler = (event: Event) => listener((event as CustomEvent<NativePlaybackState>).detail);
  window.addEventListener(eventName, handler);
  const refresh = () => {
    const saved = bridge()?.nativeGetState?.();
    if (saved) {
      try {
        listener(JSON.parse(saved) as NativePlaybackState);
      } catch {
        /* No saved session yet. */
      }
    }
  };
  window.addEventListener("focus", refresh);
  document.addEventListener("visibilitychange", refresh);
  refresh();
  return () => {
    window.removeEventListener(eventName, handler);
    window.removeEventListener("focus", refresh);
    document.removeEventListener("visibilitychange", refresh);
  };
}
