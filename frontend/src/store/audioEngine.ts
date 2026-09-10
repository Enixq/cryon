// Движок воспроизведения: связывает playerStore с реальным звуком.
//
// Две стратегии, выбираются автоматически при старте:
//  - mpv (backend): управление через App.Player* — надёжно играет любые потоки.
//  - HTML5 <audio> (фолбэк): если mpv не установлен, поток играет сам браузер
//    внутри Wails/WebView. Работает для прямых аудиоссылок и локальных файлов.
//
// Хук useAudioEngine монтируется один раз в AppLayout и подписывается на
// изменения текущего трека, паузы, громкости и перемотки.

import { useEffect, useRef } from "react";
import { usePlayerStore } from "./playerStore";
import {
  getAudioStreamUrl,
  resolvePlayableTrack,
  playerBackendAvailable,
  playerPlay,
  playerPause,
  playerResume,
  playerStop,
  playerSeek,
  playerSetVolume,
  playerSetEqualizer,
  playerSetNormalize,
  playerStatus,
  getPlaybackUrlForHtml5,
  addHistory,
  openExternal,
  listRelatedTracks,
} from "../shared/api/client";
import { notify } from "./notificationStore";
import { useEqualizerStore, EQ_FREQS } from "./equalizerStore";
import { usePlaybackSettingsStore, CROSSFADE_MS } from "./playbackSettingsStore";

export function useAudioEngine() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  // mpv установлен и доступен в принципе. Определяется один раз при старте.
  const mpvAvailableRef = useRef<boolean>(false);
  // Текущий трек реально играет через mpv (а не <audio>). Решается для
  // каждого трека отдельно: если mpv не смог — фолбэк на <audio> только
  // для этого трека, mpv остаётся доступным для следующих.
  const playingViaMpvRef = useRef<boolean>(false);
  // id трека, для которого уже загружен поток, чтобы не перезапускать зря.
  const loadedTrackIdRef = useRef<string | null>(null);
  const seekReqRef = useRef<number>(0);

  // Web Audio-граф эквалайзера/нормализации для HTML5-фолбэка. Строится лениво
  // и только один раз: createMediaElementSource можно вызвать для элемента лишь
  // единожды. Граф общий: source → compressor(нормализация) → фильтры(EQ) →
  // выход. Для mpv эквалайзер и нормализация применяются на backend
  // (playerSetEqualizer/playerSetNormalize), а не здесь.
  const audioCtxRef = useRef<AudioContext | null>(null);
  const eqFiltersRef = useRef<BiquadFilterNode[] | null>(null);
  const compressorRef = useRef<DynamicsCompressorNode | null>(null);
  // Активная анимация плавного перехода (requestAnimationFrame id) и признак
  // уже начатого затухания текущего трека — чтобы не перезапускать его каждые
  // 500 мс в тикере позиции.
  const fadeRafRef = useRef<number | null>(null);
  const fadingOutRef = useRef<boolean>(false);

  const currentIndex = usePlayerStore((s) => s.currentIndex);
  const queue = usePlayerStore((s) => s.queue);
  const isPlaying = usePlayerStore((s) => s.isPlaying);
  const volume = usePlayerStore((s) => s.volume);
  const muted = usePlayerStore((s) => s.muted);
  const seekNonce = usePlayerStore((s) => s.seekNonce);
  const seekTarget = usePlayerStore((s) => s.seekTarget);
  const eqEnabled = useEqualizerStore((s) => s.enabled);
  const eqGains = useEqualizerStore((s) => s.gains);
  const normalize = usePlaybackSettingsStore((s) => s.normalizeVolume);
  const crossfade = usePlaybackSettingsStore((s) => s.crossfade);

  const track = queue[currentIndex];

  // Целевая громкость элемента <audio> (0..1) с учётом mute — то значение, к
  // которому стремится плавное нарастание и от которого идёт затухание.
  const targetElVolume = (): number => {
    const { volume: v, muted: m } = usePlayerStore.getState();
    return m ? 0 : v / 100;
  };

  // Плавно доводит громкость элемента <audio> до target за ms миллисекунд.
  // Прерывает предыдущую анимацию. Используется для «плавного перехода»:
  // нарастание в начале трека и затухание в конце (см. CROSSFADE_MS).
  const fadeVolumeTo = (el: HTMLAudioElement, target: number, ms: number) => {
    if (fadeRafRef.current !== null) cancelAnimationFrame(fadeRafRef.current);
    const clampedTarget = Math.max(0, Math.min(1, target));
    if (ms <= 0) {
      el.volume = clampedTarget;
      fadeRafRef.current = null;
      return;
    }
    const start = el.volume;
    const t0 = performance.now();
    const step = (now: number) => {
      const p = Math.min(1, (now - t0) / ms);
      el.volume = start + (clampedTarget - start) * p;
      if (p < 1) {
        fadeRafRef.current = requestAnimationFrame(step);
      } else {
        fadeRafRef.current = null;
      }
    };
    fadeRafRef.current = requestAnimationFrame(step);
  };

  // Лениво строит Web Audio-граф для HTML5-движка: source → компрессор
  // (нормализация) → полосы эквалайзера → выход. Возвращает true, если граф
  // готов. Строится один раз; повторные вызовы — no-op. По умолчанию узлы
  // «прозрачны» (компрессор почти не сжимает, усиления полос = 0), реальные
  // параметры проставляют эффекты нормализации и эквалайзера.
  const ensureAudioGraph = (): boolean => {
    if (eqFiltersRef.current) return true;
    const el = audioRef.current;
    const Ctx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!el || !Ctx) return false;
    try {
      const ctx = new Ctx();
      const source = ctx.createMediaElementSource(el);
      const compressor = ctx.createDynamicsCompressor();
      // Прозрачные значения: пока нормализация выключена, компрессор почти не
      // влияет на звук (ratio ~1). Эффект нормализации переопределит их.
      compressor.threshold.value = 0;
      compressor.knee.value = 0;
      compressor.ratio.value = 1;
      compressor.attack.value = 0.003;
      compressor.release.value = 0.25;
      const filters = EQ_FREQS.map((freq) => {
        const node = ctx.createBiquadFilter();
        node.type = "peaking";
        node.frequency.value = freq;
        node.Q.value = 1;
        node.gain.value = 0;
        return node;
      });
      // source → compressor → f0 → f1 → … → destination
      let prev: AudioNode = source;
      prev.connect(compressor);
      prev = compressor;
      for (const node of filters) {
        prev.connect(node);
        prev = node;
      }
      prev.connect(ctx.destination);
      audioCtxRef.current = ctx;
      compressorRef.current = compressor;
      eqFiltersRef.current = filters;
      return true;
    } catch {
      // Web Audio недоступен или источник уже занят — звук продолжит играть
      // напрямую (без EQ/нормализации).
      eqFiltersRef.current = null;
      compressorRef.current = null;
      return false;
    }
  };

  // Определяем движок один раз.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const available = await playerBackendAvailable();
      if (!cancelled) mpvAvailableRef.current = available;
    })();
    // Создаём <audio> для фолбэка.
    if (!audioRef.current) {
      const el = new Audio();
      el.preload = "auto";
      // Для LAN-режима (поток с http://IP:8899) звук кросс-доменный; без CORS
      // createMediaElementSource выдал бы тишину. Сервер отдаёт заголовки CORS,
      // а тут просим анонимный запрос. На десктопе поток same-origin — атрибут
      // безвреден.
      el.crossOrigin = "anonymous";
      audioRef.current = el;
    }
    return () => {
      cancelled = true;
    };
  }, []);

  // Загрузка и запуск трека при смене currentIndex/трека.
  useEffect(() => {
    if (!track) {
      // Очередь могла стать пустой при удалении текущего трека. Явно
      // освобождаем оба движка, чтобы звук не продолжался "в фоне".
      loadedTrackIdRef.current = null;
      if (playingViaMpvRef.current) void playerStop().catch(() => {});
      playingViaMpvRef.current = false;
      const el = audioRef.current;
      if (el) {
        el.pause();
        el.onloadedmetadata = null;
        el.onerror = null;
        el.removeAttribute("src");
        el.load();
      }
      return;
    }
    if (loadedTrackIdRef.current === track.id) return;

    let cancelled = false;
    loadedTrackIdRef.current = track.id;

    // Новый трек — сбрасываем состояние плавного перехода предыдущего.
    fadingOutRef.current = false;
    if (fadeRafRef.current !== null) {
      cancelAnimationFrame(fadeRafRef.current);
      fadeRafRef.current = null;
    }

    // Освобождаем движок предыдущего трека. Без этого при смене движка
    // (HTML5 → mpv или наоборот) прежний источник продолжал звучать поверх
    // нового: <audio> никто не ставил на паузу, а mpv никто не останавливал.
    // <audio> глушим синхронно, mpv — первым делом в асинхронном блоке, до
    // playerPlay: незавершённый playerStop мог бы прилететь уже после запуска
    // нового трека и убить его.
    //
    // src снимаем, а не только паузу: эффект «пауза/воспроизведение» ниже
    // зависит от track и срабатывает сразу после этого — до того, как загрузка
    // нового потока дойдёт до конца. С прежним src он вызывал el.play(), и
    // предыдущий трек возобновлялся, а если новый уходил в mpv — продолжал
    // звучать под ним.
    const wasMpv = playingViaMpvRef.current;
    playingViaMpvRef.current = false;
    const prevEl = audioRef.current;
    if (prevEl) {
      prevEl.pause();
      prevEl.onloadedmetadata = null;
      prevEl.onerror = null;
      prevEl.removeAttribute("src");
      prevEl.load();
    }

    (async () => {
      if (wasMpv) await playerStop().catch(() => {});
      if (cancelled) return;

      // Импортированные записи могут не иметь собственного потока. До запуска
      // подбираем строгий эквивалент из приоритетных доступных источников.
      const playableTrack = await resolvePlayableTrack(track);
      if (cancelled) return;
      if (playableTrack.id !== track.id) {
        usePlayerStore.getState().replaceCurrentTrack(playableTrack);
        return;
      }

      // external_only остаётся только если альтернативный поток не найден.
      if (track.playableKind === "external_only") {
        // Не открываем внешний сервис для приостановленного трека (например,
        // восстановленного из прошлой сессии): внешнее открытие — следствие
        // явного намерения играть, а не пассивной подгрузки.
        if (!usePlayerStore.getState().isPlaying) return;
        if (track.externalUrl) {
          void openExternal(track.externalUrl).catch(() => {});
          notify({
            kind: "info",
            title: "Трек открыт во внешнем сервисе",
            message: `${track.artist} — ${track.title}: прямой поток недоступен, воспроизведение продолжится там.`,
          });
        } else {
          notify({
            kind: "warning",
            title: "Трек нельзя воспроизвести",
            message: `${track.artist} — ${track.title}: нет прямого потока и ссылки на внешний сервис.`,
          });
        }
        usePlayerStore.getState().pause();
        return;
      }

      // Запись в историю прослушивания (не блокирует воспроизведение).
      // Единственное место записи: playerStore этого не делает, иначе каждое
      // прослушивание попадало бы в базу дважды. Пишем только когда трек
      // действительно играет, а не при пассивной подгрузке (восстановление
      // сессии на паузе не должно засорять историю).
      if (usePlayerStore.getState().isPlaying) void addHistory(track).catch(() => {});
      const setEnginePosition = usePlayerStore.getState().setEnginePosition;

      // mpv играет по абсолютному пути/прямому URL (getAudioStreamUrl).
      if (mpvAvailableRef.current) {
        const mpvUrl = await getAudioStreamUrl(track.id).catch(() => null);
        if (cancelled) return;
        if (mpvUrl) {
          try {
            await playerPlay(track.id, mpvUrl);
            playingViaMpvRef.current = true;
            setEnginePosition(0);
            // Пока поток подбирался, пользователь мог нажать «Пауза»: эффект
            // пауза/воспроизведение к тому моменту видел playingViaMpvRef=false
            // и ушёл в ветку <audio>, то есть до mpv команда не дошла.
            if (!usePlayerStore.getState().isPlaying) await playerPause().catch(() => {});
            return;
          } catch {
            // Падение mpv на этом треке — фолбэк на <audio> только для него.
            // mpv остаётся доступным для следующих треков (в т.ч. из других
            // источников), иначе одна ошибка глушила бы весь сеанс.
            playingViaMpvRef.current = false;
          }
        }
      }

      // Фолбэк: HTML5 <audio>. Локальные файлы идут через /local/<id>,
      // т.к. WebView2 не проигрывает абсолютные file://-пути.
      const html5Url = await getPlaybackUrlForHtml5(track.id).catch(() => null);
      if (cancelled) return;
      const el = audioRef.current;
      if (!el) return;
      if (html5Url) {
        // Громкость читаем из стора на месте: держать её в зависимостях
        // эффекта нельзя — смена громкости во время загрузки трека отменяла
        // бы загрузку (cleanup выставляет cancelled), и трек не запускался.
        // Оттуда же берём isPlaying: пауза, нажатая пока грузился поток,
        // иначе терялась — src появлялся уже после отработки эффекта паузы.
        const { volume, muted, isPlaying: shouldPlay } = usePlayerStore.getState();
        const cf = usePlaybackSettingsStore.getState().crossfade;
        const target = muted ? 0 : volume / 100;
        el.src = html5Url;
        el.currentTime = 0;
        // Как только известны метаданные — сообщаем реальную длительность
        // (у локальных файлов теги её часто не содержат).
        const trackId = track.id;
        el.onloadedmetadata = () => {
          usePlayerStore.getState().setEngineDuration(trackId, el.duration);
        };
        // Отказ элемента раньше глушился полностью: поток не открывался, а
        // пользователь видел лишь то, что «трек не начинается». Теперь ошибка
        // видна, и плеер не остаётся в состоянии «играет».
        el.onerror = () => {
          // Для уже сменившегося трека молчим: прежний элемент может отдать
          // error после отмены загрузки.
          if (loadedTrackIdRef.current !== trackId) return;
          notify({
            kind: "warning",
            title: "Не удалось воспроизвести трек",
            message: `${track.artist} — ${track.title}: источник не отдал аудиопоток.`,
          });
          usePlayerStore.getState().pause();
        };
        if (shouldPlay) {
          // Плавный переход: стартуем с тишины и плавно нарастаем до целевой
          // громкости. Без него громкость сразу равна целевой (жёсткий старт).
          el.volume = cf ? 0 : target;
          void el
            .play()
            .then(() => {
              if (cf) fadeVolumeTo(el, target, CROSSFADE_MS);
            })
            .catch(() => {
              // Автозапуск может быть заблокирован до первого клика. Чтобы трек
              // потом не заиграл в тишину, возвращаем целевую громкость.
              el.volume = target;
            });
        } else {
          el.volume = target;
        }
      } else {
        // Нет прямого потока (embedded/external источник) — очищаем.
        el.onloadedmetadata = null;
        el.onerror = null;
        el.removeAttribute("src");
        el.load();
      }
      setEnginePosition(0);
    })();

    return () => {
      cancelled = true;
    };
  }, [track]);

  // Пауза/воспроизведение.
  useEffect(() => {
    if (!track) return;
    (async () => {
      if (playingViaMpvRef.current) {
        if (isPlaying) await playerResume().catch(() => {});
        else await playerPause().catch(() => {});
        return;
      }
      const el = audioRef.current;
      if (!el) return;
      if (isPlaying) {
        // Если звук идёт через Web Audio-граф эквалайзера, контекст мог быть
        // приостановлен — иначе элемент играет «в тишину» через граф.
        const ctx = audioCtxRef.current;
        if (ctx && ctx.state === "suspended") void ctx.resume().catch(() => {});
        if (el.src) void el.play().catch(() => {});
      } else {
        el.pause();
      }
    })();
  }, [isPlaying, track]);

  // Громкость.
  useEffect(() => {
    const v = muted ? 0 : volume;
    if (playingViaMpvRef.current) {
      void playerSetVolume(v).catch(() => {});
    }
    const el = audioRef.current;
    if (el) el.volume = v / 100;
  }, [volume, muted]);

  // Эквалайзер. Применяется к обоим движкам при изменении вкл/выкл или полос.
  useEffect(() => {
    // mpv: фильтр строится и применяется на backend. Вызываем всегда — даже
    // когда ничего не играет: контроллер запомнит фильтр и наложит его на
    // следующий Play (mpv стартует заново на каждый трек). Пустой массив при
    // выключенном EQ снимает фильтр.
    void playerSetEqualizer(eqEnabled ? eqGains : []).catch(() => {});

    // HTML5: строим Web Audio-граф лениво и только когда EQ реально включён —
    // без нужды не вставляем узлы в аудио-путь. Если граф уже построен, при
    // выключении просто обнуляем усиления (граф остаётся, звук прозрачный).
    if (!eqEnabled && !eqFiltersRef.current) return;
    if (!ensureAudioGraph()) return;

    const ctx = audioCtxRef.current;
    if (ctx && ctx.state === "suspended") void ctx.resume().catch(() => {});
    const filters = eqFiltersRef.current;
    if (filters) {
      filters.forEach((node, i) => {
        node.gain.value = eqEnabled ? (eqGains[i] ?? 0) : 0;
      });
    }
  }, [eqEnabled, eqGains]);

  // Нормализация громкости. mpv: применяется на backend (dynaudnorm), вызываем
  // всегда — контроллер запомнит флаг и наложит на следующий Play. HTML5:
  // компрессор в Web Audio-графе. Граф строим только когда нормализация реально
  // включалась (иначе не трогаем аудио-путь); при выключении делаем компрессор
  // прозрачным (ratio≈1), граф остаётся.
  useEffect(() => {
    void playerSetNormalize(normalize).catch(() => {});

    if (!normalize && !compressorRef.current) return;
    if (!ensureAudioGraph()) return;

    const ctx = audioCtxRef.current;
    if (ctx && ctx.state === "suspended") void ctx.resume().catch(() => {});
    const comp = compressorRef.current;
    if (comp) {
      if (normalize) {
        // Мягкое выравнивание: подтягиваем тихие участки и ограничиваем пики,
        // приближая воспринимаемую громкость разных треков друг к другу.
        comp.threshold.value = -24;
        comp.knee.value = 30;
        comp.ratio.value = 6;
        comp.attack.value = 0.003;
        comp.release.value = 0.25;
      } else {
        // Прозрачный режим: ratio=1 — компрессор практически не влияет на звук.
        comp.threshold.value = 0;
        comp.knee.value = 0;
        comp.ratio.value = 1;
      }
    }
  }, [normalize]);

  // Перемотка по запросу из стора (seekNonce меняется при каждом seek).
  useEffect(() => {
    if (seekNonce === seekReqRef.current) return;
    seekReqRef.current = seekNonce;
    if (playingViaMpvRef.current) {
      void playerSeek(seekTarget).catch(() => {});
    } else {
      const el = audioRef.current;
      if (el && el.src && Number.isFinite(seekTarget)) {
        el.currentTime = seekTarget;
        // Перемотка отменяет активное затухание конца трека: после прыжка
        // назад громкость нужно вернуть, иначе трек продолжит играть тихо.
        if (fadingOutRef.current) {
          fadingOutRef.current = false;
          if (fadeRafRef.current !== null) {
            cancelAnimationFrame(fadeRafRef.current);
            fadeRafRef.current = null;
          }
          el.volume = targetElVolume();
        }
      }
    }
    usePlayerStore.getState().setEnginePosition(seekTarget);
  }, [seekNonce, seekTarget]);

  // Тикер позиции: источник — mpv или <audio>. Обновляет прогресс в сторе.
  useEffect(() => {
    if (!isPlaying || !track) return;

    // Обработка конца трека с учётом режима повтора.
    const handleTrackEnd = async (store: ReturnType<typeof usePlayerStore.getState>) => {
      if (store.repeat === "one") {
        // Повтор одного трека: перематываем в начало и играем снова.
        // next() при repeat==="one" не меняет индекс, поэтому эффект
        // загрузки не сработает — перезапускаем воспроизведение вручную.
        store.setEnginePosition(0);
        if (playingViaMpvRef.current) {
          await playerSeek(0).catch(() => {});
          await playerResume().catch(() => {});
        } else if (audioRef.current) {
          // Повтор трека после возможного затухания в конце — возвращаем
          // громкость и сбрасываем признак затухания.
          fadingOutRef.current = false;
          if (fadeRafRef.current !== null) {
            cancelAnimationFrame(fadeRafRef.current);
            fadeRafRef.current = null;
          }
          audioRef.current.volume = targetElVolume();
          audioRef.current.currentTime = 0;
          void audioRef.current.play().catch(() => {});
        }
        return;
      }
      // Конец очереди без повтора: next() оставит тот же индекс. Тогда не
      // перезагружаем трек (иначе он заиграл бы заново), а останавливаемся.
      const prevIndex = store.currentIndex;
      const isLast = prevIndex >= store.queue.length - 1;
      if (isLast && store.repeat === "off" && !store.shuffle) {
        // Режим радио: дозаполняем очередь треками, похожими на текущий,
        // и продолжаем воспроизведение. Если похожих нет — останавливаемся.
        if (store.radio) {
          const related = await listRelatedTracks(track, 15).catch(() => []);
          if (related.length > 0) {
            store.appendToQueue(related);
            loadedTrackIdRef.current = null;
            store.next();
            return;
          }
        }
        store.pause();
        store.setEnginePosition(track.duration || 0);
        if (playingViaMpvRef.current) await playerStop().catch(() => {});
        return;
      }
      // Иначе разрешаем повторную загрузку и переходим к следующему.
      loadedTrackIdRef.current = null;
      store.next();
    };

    // Тик асинхронный (IPC к mpv), а интервал — 500 мс. Без защиты от
    // повторного входа два тика могли наложиться и обработать конец трека
    // дважды: очередь перескакивала через трек.
    let ticking = false;
    const id = window.setInterval(async () => {
      if (ticking) return;
      ticking = true;
      try {
        const store = usePlayerStore.getState();
        if (playingViaMpvRef.current) {
          const status = await playerStatus();
          if (status?.eof) {
            await handleTrackEnd(store);
            return;
          }
          if (status) {
            // Длительность сетевых потоков (YouTube/Yandex) известна только
            // после старта mpv — сообщаем её в стор, иначе полоска времени,
            // перемотка и форма волны не работают (duration=0 из поиска).
            if (status.durationS > 0) store.setEngineDuration(track.id, status.durationS);
            store.setEnginePosition(status.positionS);
          }
          return;
        }
        const el = audioRef.current;
        if (!el) return;
        if (el.ended) {
          await handleTrackEnd(store);
        } else {
          store.setEnginePosition(el.currentTime);
          // Плавный переход: у встроенного плеера за CROSSFADE_MS до конца
          // трека начинаем затухание. Запускаем один раз (fadingOutRef), иначе
          // тикер каждые 500 мс перезапускал бы анимацию с текущего уровня.
          if (
            usePlaybackSettingsStore.getState().crossfade &&
            !fadingOutRef.current &&
            Number.isFinite(el.duration) &&
            el.duration > 0
          ) {
            const remainingMs = (el.duration - el.currentTime) * 1000;
            if (remainingMs <= CROSSFADE_MS) {
              fadingOutRef.current = true;
              fadeVolumeTo(el, 0, remainingMs);
            }
          }
        }
      } finally {
        ticking = false;
      }
    }, 500);

    return () => window.clearInterval(id);
  }, [isPlaying, track]);

  // Остановка звука при размонтировании.
  useEffect(() => {
    return () => {
      if (fadeRafRef.current !== null) cancelAnimationFrame(fadeRafRef.current);
      if (playingViaMpvRef.current) void playerStop().catch(() => {});
      const el = audioRef.current;
      if (el) {
        el.pause();
        el.removeAttribute("src");
      }
      const ctx = audioCtxRef.current;
      if (ctx) void ctx.close().catch(() => {});
    };
  }, []);
}
