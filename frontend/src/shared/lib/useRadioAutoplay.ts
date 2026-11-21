// Упреждающее дозаполнение радио-очереди («умная очередь» играет бесконечно).
//
// Прежняя логика запрашивала похожие треки в МОМЕНТ, когда очередь уже
// закончилась: плеер останавливался и ждал ответа Last.fm/поиска (секунды),
// из-за чего в конце очереди была пауза. Теперь хвост очереди поддерживается
// заранее: как только впереди остаётся меньше TAIL_TARGET треков, фоном
// запрашивается новая порция и добавляется в конец. Пока играет трек N, треки
// N+7…N+18 уже известны, и переходы происходят мгновенно.
//
// Якорь запроса — ПОСЛЕДНИЙ трек очереди, а не текущий: каждая новая порция
// становится основой следующей, поэтому цепочка развивается, а не зацикливается
// на одном треке.

import { useEffect } from "react";
import { usePlayerStore } from "../../store/playerStore";
import { rankRadioTracks } from "../../store/radioSession";
import { listRelatedTracks } from "../api/client";
import { hasNativeAndroidQueue } from "./nativeAndroidPlayer";

/** Сколько треков держим в запасе впереди текущего. */
const TAIL_TARGET = 6;
/** Размер одной порции рекомендаций. */
const BATCH_SIZE = 24;
/** Пауза после пустого ответа, чтобы не долбить сервис рекомендаций. */
const RETRY_DELAY_MS = 5000;
/** Резервный опрос: очередь могла не меняться, а запас уже на исходе. */
const POLL_MS = 1000;

export function startRadioAutoplay(): () => void {
  let inflight = false;
  let retryAt = 0;
  let cancelled = false;
  let anchorOffset = 0;
  let session = usePlayerStore.getState().radio;

  const pump = () => {
    const state = usePlayerStore.getState();
    if (state.radio !== session) {
      session = state.radio;
      retryAt = 0;
      anchorOffset = 0;
    }
    if (inflight || Date.now() < retryAt || hasNativeAndroidQueue()) return;
    if (!state.radio || !state.playbackOnThisDevice || state.queue.length === 0) return;
    if (state.queue.length - 1 - state.currentIndex > TAIL_TARGET) return;
    const anchor =
      state.queue[
        Math.max(0, state.queue.length - 1 - (anchorOffset % Math.min(state.queue.length, 6)))
      ];
    const originalCurrent = state.queue[state.currentIndex];
    inflight = true;
    // listRelatedTracks сам глотает сетевые ошибки и отдаёт пустой список,
    // поэтому здесь достаточно обработать «рекомендаций нет».
    void listRelatedTracks(anchor, BATCH_SIZE)
      .then((related) => {
        if (cancelled) return;
        const latest = usePlayerStore.getState();
        if (
          !latest.radio ||
          !latest.playbackOnThisDevice ||
          !latest.queue.some((track) => track.id === originalCurrent.id)
        )
          return;
        const known = new Set(latest.queue.map((track) => track.id));
        const fresh = rankRadioTracks(related)
          .filter((track) => !known.has(track.id))
          .slice(0, 12);
        if (fresh.length === 0) {
          retryAt = Date.now() + RETRY_DELAY_MS;
          anchorOffset++;
          return;
        }
        anchorOffset = 0;
        if (latest.currentIndex > 100)
          usePlayerStore.setState({
            queue: latest.queue.slice(latest.currentIndex - 100),
            currentIndex: 100,
          });
        latest.appendToQueue(fresh);
        if (usePlayerStore.getState().radioWaiting) usePlayerStore.getState().next();
      })
      .catch(() => {
        retryAt = Date.now() + RETRY_DELAY_MS;
        anchorOffset++;
      })
      .finally(() => {
        inflight = false;
      });
  };

  const unsubscribe = usePlayerStore.subscribe(pump);
  const timer = window.setInterval(pump, POLL_MS);
  pump();
  return () => {
    cancelled = true;
    unsubscribe();
    window.clearInterval(timer);
  };
}

export function useRadioAutoplay(): void {
  useEffect(startRadioAutoplay, []);
}
