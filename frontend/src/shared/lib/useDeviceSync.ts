import { useEffect } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { configuredBaseUrl, setConfiguredBaseUrl } from "./httpBridge";
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

const disabledKey = "cryon.device.syncDisabled";
let confirmedPlaybackOwner = "";
function resetDisconnectedPlayer(): void {
  confirmedPlaybackOwner = "";
  localStorage.setItem(disabledKey, "1");
  configureNativeAndroidSync({ enabled: false });
  usePlayerStore.setState({ isPlaying: false, playbackDeviceId: "host", playbackOnThisDevice: true, pairedDeviceId: "" });
  setConfiguredBaseUrl("");
}

export async function disconnectDeviceSync(): Promise<boolean> {
  const peer = configuredBaseUrl();
  const base = peer || apiV1BaseUrl();
  const paired = usePlayerStore.getState().pairedDeviceId;
  localStorage.setItem(disabledKey, "1");
  configureNativeAndroidSync({ enabled: false });
  await stopLocalPlayback();
  resetDisconnectedPlayer();
  try {
    const response = await fetch(base + "/v1/devices/state", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "disconnect", deviceId: localDeviceId(), peerDeviceId: peer ? "host" : paired, isHost: !peer }), signal: AbortSignal.timeout(5000) });
    return response.ok;
  } catch { return false; }
}

export async function enableDeviceSync(): Promise<void> {
  const base = apiV1BaseUrl() || "http://127.0.0.1:8899";
  const response = await fetch(base + "/v1/devices/state", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "connect", deviceId: localDeviceId() }), signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error("Не удалось включить управление по Wi-Fi.");
  localStorage.removeItem(disabledKey);
  setConfiguredBaseUrl(configuredBaseUrl());
}

export async function selectPlaybackDevice(local: boolean): Promise<void> {
  const peer = configuredBaseUrl();
  const other = peer ? "host" : usePlayerStore.getState().pairedDeviceId;
  if (!local && !other) throw new Error("Сначала подключите другое устройство.");
  const target = local ? (peer ? localDeviceId() : "host") : other || "host";
  usePlayerStore.setState({
    playbackDeviceId: target,
  });
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const state = usePlayerStore.getState();
    if (confirmedPlaybackOwner === target && state.playbackDeviceId === target && state.playbackOnThisDevice === local) return;
    await new Promise(resolve => window.setTimeout(resolve, 100));
  }
  throw new Error("Устройство не подтвердило переключение. Проверьте сеть или отключите синхронизацию.");
}

export function startDeviceSync(): () => void {
  if (!hasApiV1Runtime()) return () => {};
  if (localStorage.getItem(disabledKey) === "1") return () => {};
  const peer = configuredBaseUrl();
  const base = peer || apiV1BaseUrl() || "http://127.0.0.1:8899";
  const id = localDeviceId();
  confirmedPlaybackOwner = "";
  // Silence a newly connected controller before receiving the first snapshot.
  if (peer) usePlayerStore.setState({ playbackOnThisDevice: false, playbackDeviceId: "host" });
  let revision = 0;
  let applying = false;
  let initialized = false;
  let stopped = false;
  let busy = false;
  let rerun = false;
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
      radioSeed: s.radioSeed,
      repeat: s.repeat,
      shuffle: s.shuffle,
      seekNonce: s.seekRevision,
      playbackDeviceId: s.playbackDeviceId,
    };
  };
  const signature = () => JSON.stringify({ ...snapshot(), progress: undefined });
  let pendingIntent:
    { desired: ReturnType<typeof snapshot>; baseline: ReturnType<typeof snapshot> } | undefined;
  const tick = async () => {
    if (busy || stopped || document.visibilityState === "hidden") return;
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
        detachedDeviceIds?: string[];
      };
      if (stopped) return;
      if (incoming.detachedDeviceIds?.includes(id)) {
        await stopLocalPlayback();
        resetDisconnectedPlayer();
        return;
      }
      // A user command may have arrived while the GET/PUT was in flight.
      // Rebase that intent over the reply instead of replacing it with it.
      if (initialized && lastSignature && signature() !== lastSignature && !pendingIntent)
        pendingIntent = { desired: snapshot(), baseline: JSON.parse(lastSignature) as ReturnType<typeof snapshot> };
      activeOwner = incoming.activePlaybackDeviceId ?? incoming.playbackDeviceId ?? "host";
      releaseDeviceId = incoming.releaseDeviceId || "";
      confirmedPlaybackOwner = releaseDeviceId ? "" : activeOwner;
      if (
        Array.isArray(incoming.queue) &&
        incoming.deviceId &&
        (!initialized || Number(incoming.revision) > revision)
      ) {
        revision = Number(incoming.revision) || 0;
        applying = true;
        const owner = incoming.playbackDeviceId || "host";
        if (!peer && incoming.deviceId !== id)
          usePlayerStore.setState({ pairedDeviceId: incoming.detachedDeviceIds?.includes(incoming.deviceId) ? "" : incoming.deviceId });
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
          "radioSeed",
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
          patch.seekRevision = usePlayerStore.getState().seekRevision + 1;
          patch.remoteSeekNonce = patch.seekRevision;
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
          body: JSON.stringify({ releaseDeviceId }),
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
      if (rerun && !stopped) { rerun = false; queueMicrotask(() => void tick()); }
    }
  };
  const off = usePlayerStore.subscribe((state, previous) => {
    // Position/duration observations are rendered locally. Only semantic
    // changes request an immediate sync; position uses the timed heartbeat.
    if (!applying && (state.queue !== previous.queue || state.currentIndex !== previous.currentIndex ||
      state.isPlaying !== previous.isPlaying || state.volume !== previous.volume || state.muted !== previous.muted ||
      state.radio !== previous.radio || state.radioSeed !== previous.radioSeed || state.repeat !== previous.repeat || state.shuffle !== previous.shuffle ||
      state.seekRevision !== previous.seekRevision || state.playbackDeviceId !== previous.playbackDeviceId)) { if (busy) rerun = true; else void tick(); }
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
