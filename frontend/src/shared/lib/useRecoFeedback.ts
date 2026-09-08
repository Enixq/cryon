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
    mutationFn: ({ artist, title, score }: { artist: string; title: string; score: number }) =>
      setRecoFeedback(artist, title, score),
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
    },
    onSuccess: (map) => {
      queryClient.setQueryData(recoFeedbackQueryKey, map);
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

  const scoreFor = useCallback(
    (track: Track) => state?.[recoFeedbackKey(track.artist, track.title)] ?? 0,
    [state],
  );

  // mutate стабилен между рендерами (в отличие от объекта мутации целиком),
  // поэтому rate не пересоздаётся на каждый рендер.
  const mutate = mutation.mutate;
  const rate = useCallback(
    (track: Track, score: 1 | -1) => {
      const current = state?.[recoFeedbackKey(track.artist, track.title)] ?? 0;
      // Повторный клик по уже нажатой кнопке — снятие оценки.
      const next = current === score ? 0 : score;
      mutate({ artist: track.artist, title: track.title, score: next });

      // Синхронизация с избранным: «нравится» = сохранить трек в библиотеку,
      // снятие лайка (в т.ч. переключение на «не нравится») — убрать. Дизлайк
      // сам по себе избранного не трогает. Событие favorites:changed обновит
      // «Мне нравится» и сердечки в плеере; ошибку показываем пользователю,
      // иначе трек молча не попал бы в библиотеку (баг, который и чинится).
      const wasLiked = current === 1;
      const willLike = next === 1;
      if (willLike && !wasLiked) {
        void addFavorite(track).catch(() =>
          notify({
            kind: "error",
            title: "Не удалось добавить в избранное",
            message: `${track.artist} — ${track.title}`,
          }),
        );
      } else if (!willLike && wasLiked) {
        void removeFavorite(track.id).catch(() =>
          notify({
            kind: "error",
            title: "Не удалось убрать из избранного",
            message: `${track.artist} — ${track.title}`,
          }),
        );
      }
    },
    [mutate, state],
  );

  return { scoreFor, rate, saving: mutation.isPending };
}
