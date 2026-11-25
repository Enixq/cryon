import { usePlayerStore } from "../../store/playerStore";
import { reportDJEvent } from "../api/intelligence";
// Оценки рекомендаций («нравится»/«не нравится» на карточке) — обратная связь
// в профиль вкусов. Backend хранит их в настройках и учитывает при следующей
// сборке подборки: плюс усиливает исполнителя, накопленный минус исключает его,
// оценённый трек больше не предлагается (см. internal/recommendations).
//
// «Нравится» на карточке одновременно кладёт трек в избранное («Мне нравится»):
// для пользователя лайк — это «сохранить к себе», странно было бы, если бы
// одобренный трек не появлялся в библиотеке. Снятие лайка (повторный клик или
// переключение на «не нравится») убирает трек из избранного.

import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  recoFeedbackKey,
  recoFeedbackState,
  setRecoFeedback,
  addFavorite,
  removeFavorite,
  type RecoFeedbackMap,
} from "../api/client";
import { notify } from "../../store/notificationStore";
import type { Track } from "../types";

/** Ключ кэша оценок. Экспортируем, чтобы страницы могли его сбрасывать. */
export const recoFeedbackQueryKey = ["recoFeedback"] as const;

export interface RecoFeedback {
  /** Оценка трека: 1 — нравится, -1 — не нравится, 0 — не оценён. */
  scoreFor: (track: Track) => number;
  /** Поставить оценку. Повторный клик по той же кнопке снимает её. */
  rate: (track: Track, score: 1 | -1) => void;
  /** Идёт сохранение оценки — кнопки блокируются, чтобы не двоить запросы. */
  saving: boolean;
  artistBlocked: (track: Track) => boolean;
  toggleArtist: (track: Track) => void;
  blockedArtists: string[];
  allowArtist: (artist: string) => void;
}

// Purge existing queues too; a successful block must take effect before refill.
function purgeBlockedQueue(map: RecoFeedbackMap): void {
  const allowed = (track: Track) => {
    const artists = [track.primaryArtist || track.artist, ...(track.artists ?? [])];
    return (
      !artists.some((a) => (map[recoFeedbackKey(a, "*")] ?? 0) < 0) &&
      (map[recoFeedbackKey(track.primaryArtist || track.artist, track.title)] ?? 0) >= 0
    );
  };
  const state = usePlayerStore.getState();
  const queue = state.queue.filter(allowed);
  if (queue.length === state.queue.length) return;
  const target = state.queue.slice(state.currentIndex).find(allowed) ?? queue[queue.length - 1];
  usePlayerStore.setState({
    queue,
    currentIndex: Math.max(
      0,
      queue.findIndex((t) => t.id === target?.id),
    ),
    ...(queue.length ? {} : { isPlaying: false }),
  });
}

export function useRecoFeedback(): RecoFeedback {
  const queryClient = useQueryClient();

  // Оценок немного (десятки пар) и меняются они только отсюда, поэтому
  // перезапрашивать их по времени не нужно.
  const { data: state } = useQuery<RecoFeedbackMap>({
    queryKey: recoFeedbackQueryKey,
    queryFn: recoFeedbackState,
    staleTime: Infinity,
  });

  const mutation = useMutation({
    mutationFn: ({
      artist,
      title,
      score,
    }: {
      artist: string;
      title: string;
      score: number;
      track: Track;
      wasLiked: boolean;
    }) => setRecoFeedback(artist, title, score),
    // Оптимистично обновляем карту: кнопка должна закрашиваться сразу, а не
    // после ответа backend (оценка идёт в SQLite, это заметная задержка).
    onMutate: ({ artist, title, score }) => {
      const key = recoFeedbackKey(artist, title);
      const prev = queryClient.getQueryData<RecoFeedbackMap>(recoFeedbackQueryKey) ?? {};
      const next = { ...prev };
      if (score === 0) delete next[key];
      else next[key] = score;
      queryClient.setQueryData(recoFeedbackQueryKey, next);
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(recoFeedbackQueryKey, context.prev);
      notify({
        kind: "error",
        title: "Не удалось сохранить оценку",
        message: "Попробуйте ещё раз.",
      });
    },
    onSuccess: (map, { track, score, wasLiked }) => {
      queryClient.setQueryData(recoFeedbackQueryKey, map);
      purgeBlockedQueue(map);
      if (score !== 0) reportDJEvent(track, score === 1 ? "like" : "dislike");
      if (score === 1 && !wasLiked) {
        void addFavorite(track).catch(() =>
          notify({
            kind: "error",
            title: "Не удалось добавить в избранное",
            message: `${track.artist} — ${track.title}`,
          }),
        );
      } else if (score !== 1 && wasLiked) {
        void removeFavorite(track.id).catch(() =>
          notify({
            kind: "error",
            title: "Не удалось убрать из избранного",
            message: `${track.artist} — ${track.title}`,
          }),
        );
      }
      // Подборки помечаем устаревшими, но НЕ перезапрашиваем сразу: пересборка
      // на backend — это десятки поисков по всем источникам, а пользователь
      // обычно оценивает несколько карточек подряд. Новая версия оценок входит
      // в ключ кэша подборки, поэтому следующий заход соберёт её заново.
      // «Радар новинок» тоже зависит от профиля — он упорядочивается по
      // знакомым исполнителям (Engine.PersonalizeReleases).
      queryClient.invalidateQueries({ queryKey: ["recommendations"], refetchType: "none" });
      queryClient.invalidateQueries({ queryKey: ["autoMix"], refetchType: "none" });
      queryClient.invalidateQueries({ queryKey: ["collection"], refetchType: "none" });
      queryClient.invalidateQueries({ queryKey: ["newReleases"], refetchType: "none" });
    },
  });

  const artistBlocked = useCallback(
    (track: Track) =>
      [track.primaryArtist || track.artist, ...(track.artists ?? [])].some(
        (artist) => (state?.[recoFeedbackKey(artist, "*")] ?? 0) < 0,
      ),
    [state],
  );
  const artistMutation = useMutation({
    mutationFn: ({ artist, blocked }: { artist: string; blocked: boolean }) =>
      setRecoFeedback(artist, "*", blocked ? -1 : 0),
    onSuccess: (map) => {
      queryClient.setQueryData(recoFeedbackQueryKey, map);
      purgeBlockedQueue(map);
      queryClient.invalidateQueries({ queryKey: ["collection"], refetchType: "none" });
    },
    onError: () => notify({ kind: "error", title: "Не удалось сохранить запрет исполнителя" }),
  });
  const scoreFor = useCallback(
    (track: Track) =>
      artistBlocked(track)
        ? -1
        : (state?.[recoFeedbackKey(track.primaryArtist || track.artist, track.title)] ?? 0),
    [state, artistBlocked],
  );

  // mutate стабилен между рендерами (в отличие от объекта мутации целиком),
  // поэтому rate не пересоздаётся на каждый рендер.
  const mutate = mutation.mutate;
  const rate = useCallback(
    (track: Track, score: 1 | -1) => {
      const artist = track.primaryArtist || track.artist;
      const current = state?.[recoFeedbackKey(artist, track.title)] ?? 0;
      // Повторный клик по уже нажатой кнопке — снятие оценки.
      const next = current === score ? 0 : score;
      mutate({ artist, title: track.title, score: next, track, wasLiked: current === 1 });
    },
    [mutate, state],
  );

  return {
    scoreFor,
    rate,
    artistBlocked,
    toggleArtist: (track) =>
      artistMutation.mutate({
        artist: track.primaryArtist || track.artist,
        blocked: !artistBlocked(track),
      }),
    blockedArtists: Object.keys(state ?? {})
      .filter((k) => k.endsWith("|*") && (state?.[k] ?? 0) < 0)
      .map((k) => k.slice(0, -2)),
    allowArtist: (artist) => artistMutation.mutate({ artist, blocked: false }),
    saving: mutation.isPending || artistMutation.isPending,
  };
}
