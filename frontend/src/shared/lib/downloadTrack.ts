import type { Track } from "../types";
import { splitTrackId } from "../api/mappers";
import { apiV1BaseUrl, hasApiV1Runtime, request } from "../api/v1";

export async function downloadTrack(track: Track): Promise<boolean> {
  if (!hasApiV1Runtime()) throw new Error("Скачивание доступно в приложении Cryon");
  const { source, rawId } = splitTrackId(track.id);
  const params = new URLSearchParams({
    service: source,
    id: rawId,
    name: `${track.artist} — ${track.title}`,
  });
  const path = `/v1/library/export?${params}`;
  if (window.CryonAndroid?.nativeDownload && window.CryonAndroid.nativeDownloadStatus) {
    const ticket = window.CryonAndroid.nativeDownload(apiV1BaseUrl() + path);
    for (let i = 0; i < 900; i++) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      const status = JSON.parse(window.CryonAndroid.nativeDownloadStatus(ticket)) as {
        state: string;
        error?: string;
      };
      if (status.state === "saved") return true;
      if (status.state === "cancelled") return false;
      if (status.state === "error") throw new Error(status.error || "Не удалось сохранить трек");
    }
    throw new Error("Скачивание не завершено");
  }
  const result = await request<{ saved: boolean }>(path, { method: "POST" });
  return result.saved;
}
