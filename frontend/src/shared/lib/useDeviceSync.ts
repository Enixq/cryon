import { useEffect } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { configuredBaseUrl } from "./httpBridge";
import { apiV1BaseUrl, hasApiV1Runtime } from "../api/v1";
import { configureNativeAndroidSync } from "./nativeAndroidPlayer";
import { stopLocalPlayback } from "./localPlayback";

const volatileDeviceId = `device-${crypto.randomUUID?.() || Math.random().toString(36).slice(2)}`;
export function localDeviceId(): string {
  const key = "cryon.device.id";
  try {
    let id = localStorage.getItem(key);
    if (!id) {
      id = volatileDeviceId;
      localStorage.setItem(key, id);
    }
    return id;
  } catch {
    return volatileDeviceId;
  }
}

/** A client controls the host by default, and can explicitly take playback. */
export function ownsPlayback(owner: string, id: string, connectedToPeer: boolean): boolean {
  return owner === id || (owner === "host" && !connectedToPeer);
}

export function selectPlaybackDevice(local: boolean): void {
  const peer = configuredBaseUrl();
  const other = peer ? "host" : usePlayerStore.getState().pairedDeviceId;
  usePlayerStore.setState({
    playbackDeviceId: local ? (peer ? localDeviceId() : "host") : other || "host",
  });
}

export function startDeviceSync(): () => void {
  if (!hasApiV1Runtime()) return () => {};
  const peer = configuredBaseUrl();
  const base = peer || apiV1BaseUrl() || "http://127.0.0.1:8899";
  const id = localDeviceId();
  // Silence a newly connected controller before receiving the first snapshot.
  if (peer) usePlayerStore.setState({ playbackOnThisDevice: false, playbackDeviceId: "host" });
  let revision = 0;
  let applying = false;
  let initialized = false;
  let stopped = false;
  let busy = false;
  let lastSignature = "";
  let activeOwner = "host";
  let releaseDeviceId = "";
  let lastPublishAt = 0;
  let activeRequest: AbortController | undefined;
  const request = async (init?: RequestInit) => {
    const controller = new AbortController();
    activeRequest = controller;
    const timeout = window.setTimeout(() => controller.abort(), 5000);
    try {
      return await fetch(base + "/v1/devices/state", { ...init, signal: controller.signal });
    } finally {
      window.clearTimeout(timeout);
      if (activeRequest === controller) activeRequest = undefined;
    }
  };
  const snapshot = () => {
    const s = usePlayerStore.getState();
    return {
      queue: s.queue,
      currentIndex: s.currentIndex,
      isPlaying: s.isPlaying,
      progress: s.progress,
      volume: s.volume,
      muted: s.muted,
      radio: s.radio,
      repeat: s.repeat,
      shuffle: s.shuffle,
      seekNonce: s.seekNonce,
      playbackDeviceId: s.playbackDeviceId,
    };
  };
  const signature = () => JSON.stringify({ ...snapshot(), progress: undefined });
  let pendingIntent:
    { desired: ReturnType<typeof snapshot>; baseline: ReturnType<typeof snapshot> } | undefined;
  const tick = async () => {
    if (busy || stopped) return;
    busy = true;
    try {
      const s = usePlayerStore.getState();
      const changed =
        initialized &&
        (signature() !== lastSignature ||
          (s.playbackOnThisDevice && s.isPlaying && Date.now() - lastPublishAt >= 1000));
      if (changed) {
        const sent = signature();
        const desired = snapshot();
        const response = await request({
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ...desired, deviceId: id, baseRevision: revision }),
        });
        if (stopped) return;
        if (response.ok) {
          const ack = (await response.json()) as { revision: number };
          revision = ack.revision;
          lastSignature = sent;
          lastPublishAt = Date.now();
        } else if (response.status === 409 && lastSignature)
          pendingIntent = {
            desired,
            baseline: JSON.parse(lastSignature) as ReturnType<typeof snapshot>,
          };
      }
      const response = await request();
      if (!response.ok || stopped) return;
      const incoming = (await response.json()) as ReturnType<typeof snapshot> & {
        revision?: number;
        deviceId?: string;
        activePlaybackDeviceId?: string;
        releaseDeviceId?: string;
      };
      if (stopped) return;
      activeOwner = incoming.activePlaybackDeviceId ?? incoming.playbackDeviceId ?? "host";
      releaseDeviceId = incoming.releaseDeviceId || "";
      if (
        Array.isArray(incoming.queue) &&
        incoming.deviceId &&
        (!initialized || Number(incoming.revision) > revision)
      ) {
        revision = Number(incoming.revision) || 0;
        applying = true;
        const owner = incoming.playbackDeviceId || "host";
        if (!peer && incoming.deviceId !== id)
          usePlayerStore.setState({ pairedDeviceId: incoming.deviceId });
        usePlayerStore
          .getState()
          .replaceRemoteState({
            ...incoming,
            playbackDeviceId: owner,
            playbackOnThisDevice:
              !releaseDeviceId && activeOwner === owner && ownsPlayback(owner, id, Boolean(peer)),
          });
        applying = false;
        lastSignature = signature();
      }
      if (pendingIntent) {
        const { desired, baseline } = pendingIntent;
        pendingIntent = undefined;
        const patch: Partial<ReturnType<typeof usePlayerStore.getState>> = {};
        for (const key of [
          "queue",
          "isPlaying",
          "volume",
          "muted",
          "radio",
          "repeat",
          "shuffle",
          "playbackDeviceId",
        ] as const) {
          if (JSON.stringify(desired[key]) !== JSON.stringify(baseline[key]))
            Object.assign(patch, { [key]: desired[key] });
        }
        if (desired.currentIndex !== baseline.currentIndex) {
          const current = usePlayerStore.getState();
          patch.currentIndex = Math.max(
            0,
            Math.min(
              (patch.queue || current.queue).length - 1,
              patch.queue
                ? desired.currentIndex
                : current.currentIndex + desired.currentIndex - baseline.currentIndex,
            ),
          );
          patch.progress = 0;
          patch.engineDuration = 0;
        }
        if (desired.seekNonce !== baseline.seekNonce) {
          patch.seekTarget = desired.progress;
          patch.progress = desired.progress;
          patch.seekNonce = usePlayerStore.getState().seekNonce + 1;
        }
        applying = true;
        usePlayerStore.setState(patch);
        applying = false;
      }
      if (!initialized) {
        initialized = true;
        if (!incoming.deviceId) {
          // The host seeds the shared queue. Controllers wait for it.
          lastSignature = peer ? signature() : "";
        }
      }
      const state = usePlayerStore.getState();
      const local =
        !releaseDeviceId &&
        activeOwner === state.playbackDeviceId &&
        ownsPlayback(state.playbackDeviceId, id, Boolean(peer));
      if (state.playbackOnThisDevice !== local) {
        applying = true;
        usePlayerStore.setState({ playbackOnThisDevice: local });
        applying = false;
      }
      configureNativeAndroidSync({
        baseUrl: base,
        deviceId: id,
        isHost: !peer,
        revision,
        ...snapshot(),
        playbackOnThisDevice: local,
      });
      if (releaseDeviceId && ownsPlayback(releaseDeviceId, id, Boolean(peer))) {
        await stopLocalPlayback();
        if (stopped) return;
        const released = await request({
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ releaseDeviceId, baseRevision: revision }),
        });
        if (released.ok) {
          const ack = (await released.json()) as { revision: number };
          revision = ack.revision - 1;
        }
      }
    } catch {
      /* Keep the selected owner during transient network failures. */
    } finally {
      busy = false;
    }
  };
  const off = usePlayerStore.subscribe(() => {
    if (!applying) void tick();
  });
  const timer = window.setInterval(() => {
    void tick();
  }, 1000);
  void tick();
  return () => {
    stopped = true;
    activeRequest?.abort();
    off();
    window.clearInterval(timer);
  };
}

export function useDeviceSync(connectionRevision: number): void {
  useEffect(startDeviceSync, [connectionRevision]);
}
