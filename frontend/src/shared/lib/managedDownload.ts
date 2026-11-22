import type { Track } from "../types";
import { toBackendTrack } from "../api/mappers";
import { apiV1BaseUrl, hasApiV1Runtime, request, type BackgroundJob } from "../api/v1";
import { useDownloadStore } from "../../store/downloadStore";
const pending = new Map<string, Promise<boolean>>();
export function downloadTrack(track: Track): Promise<boolean> {
  const existing = pending.get(track.id);
  if (existing) return existing;
  const work = saveTrack(track).finally(() => pending.delete(track.id));
  pending.set(track.id, work);
  return work;
}
async function saveTrack(track: Track): Promise<boolean> {
  if (!hasApiV1Runtime()) throw new Error("Скачивание доступно в приложении Cryon");
  const update = (
    progress: number,
    message: string,
    state: "loading" | "saved" | "error" = "loading",
  ) =>
    useDownloadStore
      .getState()
      .update({
        id: track.id,
        title: `${track.artist} — ${track.title}`,
        progress,
        message,
        state,
      });
  update(0, "Подготавливаем полный трек…");
  try {
    const result = await request<{ key: string; saved?: boolean; jobId?: string }>(
      "/v1/library/downloads",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ track: toBackendTrack(track) }),
      },
    );
    if (!result.saved) {
      let finished = false;
      for (let i = 0; i < 1500; i++) {
        await new Promise((resolve) => setTimeout(resolve, 800));
        const jobs = await request<BackgroundJob[]>("/v1/jobs");
        const job = jobs.find((row) => row.id === result.jobId);
        if (!job) throw new Error("Скачивание не найдено");
        update(job.progress * 0.85, job.message || "Скачиваем…");
        if (job.state === "completed") {
          finished = true;
          break;
        }
        if (job.state === "failed" || job.state === "cancelled")
          throw new Error(job.error || "Скачивание отменено");
      }
      if (!finished)
        throw new Error("Загрузка продолжается в фоне. Проверьте раздел «Скачано» позже.");
    }
    const android = window.CryonAndroid;
    if (android?.nativeDownload && android.nativeDownloadStatus) {
      const ticket = android.nativeDownload(
        apiV1BaseUrl() + `/v1/library/downloads/file?key=${result.key}`,
      );
      for (let i = 0; i < 900; i++) {
        await new Promise((resolve) => setTimeout(resolve, 500));
        const status = JSON.parse(android.nativeDownloadStatus(ticket)) as {
          state: string;
          error?: string;
          received?: number;
          total?: number;
          message?: string;
        };
        update(
          0.85 + 0.15 * Math.min(1, (status.received || 0) / (status.total || 1)),
          status.message || "Сохраняем в Music/Cryon…",
        );
        if (status.state === "saved") {
          update(1, "Сохранено в Music/Cryon и доступно без сети", "saved");
          window.dispatchEvent(new Event("cryon:downloads-changed"));
          return true;
        }
        if (status.state === "error")
          throw new Error(
            `${status.error || "Не удалось экспортировать"}. Полный трек сохранён внутри Cryon в разделе «Скачано».`,
          );
      }
      throw new Error("Трек доступен внутри Cryon; копирование в Music/Cryon ещё не завершено");
    }
    update(1, "Сохранено · Локальная музыка → Скачано", "saved");
    window.dispatchEvent(new Event("cryon:downloads-changed"));
    return true;
  } catch (error) {
    update(0, (error as Error).message, "error");
    window.dispatchEvent(new Event("cryon:downloads-changed"));
    throw error;
  }
}
