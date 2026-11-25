import Hls from "hls.js";

/** HLS attaches to the same audio element, preserving EQ and transport controls. */
export function attachHls(
  audio: HTMLAudioElement,
  url: string,
  onFailure: (message: string) => void,
): (() => void) | null {
  if (audio.canPlayType("application/vnd.apple.mpegurl")) {
    audio.src = url;
    return () => {};
  }
  if (!Hls.isSupported()) {
    onFailure("Встроенный плеер не поддерживает HLS.");
    return null;
  }
  const hls = new Hls({ maxBufferLength: 30, maxMaxBufferLength: 60 });
  hls.on(Hls.Events.MEDIA_ATTACHED, () => hls.loadSource(url));
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal)
      onFailure(
        data.type === Hls.ErrorTypes.NETWORK_ERROR
          ? "ВК: не удалось загрузить сегмент аудио."
          : "ВК: не удалось декодировать аудиопоток.",
      );
  });
  hls.attachMedia(audio);
  return () => hls.destroy();
}
