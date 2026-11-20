import {
  FolderOpen,
  Check,
  CircleHelp,
  Download,
  RefreshCw,
  Music2,
  Link2,
  SlidersHorizontal,
  Settings2,
  Activity,
  Trash2,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { SOURCES, sourceName } from "../shared/sources";
import { cn } from "../shared/lib/cn";
import { formatDuration } from "../shared/lib/format";
import type { SourceId, Track } from "../shared/types";
import { ServiceIcon } from "../shared/ui/ServiceIcon";
import { PageHeader } from "../shared/ui/PageHeader";
import { TabBar, type TabItem } from "../shared/ui/TabBar";
import { useUiStore } from "../store/uiStore";
import { usePlaybackSettingsStore } from "../store/playbackSettingsStore";
import { usePlaybackDiagnosticsStore } from "../store/playbackDiagnosticsStore";
import { useFavoriteIds } from "../shared/lib/useFavorites";
import {
  addLocalFolder,
  getSetting,
  listLocalFolders,
  listSourceStatus,
  onSourceStatusChanged,
  pickMusicFolder,
  scanDeviceMusic,
  removeLocalFolder,
  setSetting,
  setYandexToken,
  yandexTokenConnected,
  startYandexLogin,
  onYandexConnected,
  setLastFMKey,
  lastfmConnected,
  resetAllData,
  setSpotifyCredentials,
  spotifyConnected,
  setSoundCloudClientID,
  soundcloudConnected,
  setYouTubeAPIKey,
  youtubeConnected,
  importFavorites,
  importSpotifyLibraryExport,
  previewSpotifyLibraryExport,
  startSpotifyLogin,
  listAccountConnections,
  discoverDevices,
  setOAuthApplicationConfig,
  validateSource,
  type FavoritesImportResult,
} from "../shared/api/client";
import { configuredBaseUrl, setConfiguredBaseUrl } from "../shared/lib/httpBridge";
import { apiV1BaseUrl, hasApiV1Runtime, playbackPreferencesV1, setPlaybackPreferencesV1, spotifyFileImportV1, type PlaybackPreferencesV1 } from "../shared/api/v1";
import { mapTracks, splitTrackId } from "../shared/api/mappers";
import { hasNativeAndroidPlayer } from "../shared/lib/nativeAndroidPlayer";
import type { domain } from "../../wailsjs/go/models";

// Ключи персистентных настроек в хранилище. Плавный переход и нормализация
// хранятся под ключами playback.crossfade / playback.normalizeVolume внутри
// playbackSettingsStore; здесь остаётся только «высокое качество».
const KEY_HIGH_QUALITY = "playback.highQuality";
const DEFAULT_PLAYBACK_PREFERENCES: PlaybackPreferencesV1 = {
  preferLocal: false,
  preferExplicit: false,
  officialOnly: false,
  sourcePriority: {},
};
const PLAYBACK_SOURCE_ORDER: SourceId[] = ["soundcloud", "youtube", "local", "yandex"];

type SettingsTab = "music" | "connections" | "playback" | "diagnostics" | "general";

const SETTINGS_TABS: TabItem<SettingsTab>[] = [
  { id: "music", label: "Музыка", icon: Music2 },
  { id: "connections", label: "Подключения", icon: Link2 },
  { id: "playback", label: "Звук", icon: SlidersHorizontal },
  { id: "diagnostics", label: "Диагностика", icon: Activity },
  { id: "general", label: "Общее", icon: Settings2 },
];

export function SettingsPage() {
  const [tab, setTab] = useState<SettingsTab>("music");
  // Плавный переход и нормализация живут в сторе: их переключение должно сразу
  // влиять на живой движок (audioEngine подписан на стор), а не только писать
  // ключ в базу. Стор сам персистит значения теми же ключами playback.*.
  const crossfade = usePlaybackSettingsStore((s) => s.crossfade);
  const setCrossfade = usePlaybackSettingsStore((s) => s.setCrossfade);
  const normalizeVolume = usePlaybackSettingsStore((s) => s.normalizeVolume);
  const setNormalizeVolume = usePlaybackSettingsStore((s) => s.setNormalizeVolume);
  const [highQuality, setHighQuality] = useState(true);
  // Пока настройки не загружены из хранилища, не пишем их обратно (чтобы не
  // затереть сохранённые значения дефолтами при первом рендере).
  const [loaded, setLoaded] = useState(false);
  const yandexConfigRef = useRef<HTMLDivElement | null>(null);
  const localConfigRef = useRef<HTMLDivElement | null>(null);
  const lastfmConfigRef = useRef<HTMLDivElement | null>(null);
  const spotifyConfigRef = useRef<HTMLDivElement | null>(null);
  const soundcloudConfigRef = useRef<HTMLDivElement | null>(null);
  const youtubeConfigRef = useRef<HTMLDivElement | null>(null);
  const queryClient = useQueryClient();
  const setEqualizerOpen = useUiStore((s) => s.setEqualizerOpen);
  const playbackDiagnostics = usePlaybackDiagnosticsStore((s) => s.items);
  const clearPlaybackDiagnostics = usePlaybackDiagnosticsStore((s) => s.clear);
  const exportPlaybackDiagnostics = () => {
    const report = {
      exportedAt: new Date().toISOString(),
      events: playbackDiagnostics.map(({ kind, title, artist, fromSource, toSource, positionSeconds, createdAt, fallbackSearchMs, startupMs }) => ({
        kind, title, artist, fromSource, toSource, positionSeconds, createdAt, fallbackSearchMs, startupMs,
      })),
    };
    const reportJson = JSON.stringify(report, null, 2);
    if (window.CryonAndroid?.savePlaybackReport) {
      window.CryonAndroid.savePlaybackReport(reportJson);
      return;
    }
    const url = URL.createObjectURL(new Blob([reportJson], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "cryon-playback-report.json";
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const { data: playbackPreferences = DEFAULT_PLAYBACK_PREFERENCES } = useQuery({
    queryKey: ["playbackPreferences"],
    queryFn: () => hasApiV1Runtime() ? playbackPreferencesV1() : Promise.resolve(DEFAULT_PLAYBACK_PREFERENCES),
  });
  const playbackPreferencesMutation = useMutation({
    mutationFn: setPlaybackPreferencesV1,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["playbackPreferences"] }),
  });
  const orderedPlaybackSources = [...PLAYBACK_SOURCE_ORDER].sort(
    (left, right) => (playbackPreferences.sourcePriority[right] ?? 0) - (playbackPreferences.sourcePriority[left] ?? 0),
  );
  const movePlaybackSource = (source: SourceId, direction: -1 | 1) => {
    const index = orderedPlaybackSources.indexOf(source);
    const nextIndex = index + direction;
    if (index < 0 || nextIndex < 0 || nextIndex >= orderedPlaybackSources.length) return;
    const next = [...orderedPlaybackSources];
    [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
    playbackPreferencesMutation.mutate({
      ...playbackPreferences,
      sourcePriority: Object.fromEntries(next.map((id, position) => [id, (next.length - position) * 10])),
    });
  };

  // Каждый источник ведёт к своей секции настройки. Раньше все варианты
  // прокручивали к Yandex — теперь у каждого сервиса собственный якорь.
  const configRefs: Partial<Record<SourceId, RefObject<HTMLDivElement | null>>> = {
    yandex: yandexConfigRef,
    local: localConfigRef,
    spotify: spotifyConfigRef,
    soundcloud: soundcloudConfigRef,
    youtube: youtubeConfigRef,
  };

  // На какой вкладке живёт секция настройки источника. Кнопка «Подключить» в
  // обзоре источников (вкладка «Музыка») переключает на нужную вкладку и уже
  // там прокручивает к секции.
  const configTab: Partial<Record<SourceId, SettingsTab>> = {
    yandex: "connections",
    spotify: "connections",
    soundcloud: "connections",
    youtube: "connections",
    local: "music",
  };

  // Отложенная прокрутка: секция может находиться на другой вкладке, которая
  // ещё не смонтирована. Сначала переключаем вкладку, затем в эффекте (когда
  // ref уже заполнен) прокручиваем к секции.
  const [pendingScroll, setPendingScroll] = useState<SourceId | null>(null);

  const focusConfigSection = (id: SourceId) => {
    const targetTab = configTab[id];
    if (targetTab && targetTab !== tab) {
      setTab(targetTab);
      setPendingScroll(id);
      return;
    }
    configRefs[id]?.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  useEffect(() => {
    if (!pendingScroll) return;
    const ref = configRefs[pendingScroll];
    if (ref?.current) {
      ref.current.scrollIntoView({ behavior: "smooth", block: "start" });
      setPendingScroll(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, pendingScroll]);

  const { data: folders = [] } = useQuery({
    queryKey: ["localFolders"],
    queryFn: listLocalFolders,
  });

  const { data: services = [], isLoading: servicesLoading = false, refetch: refetchSourceStatus } = useQuery({
    queryKey: ["sourceStatus"],
    queryFn: listSourceStatus,
    staleTime: 5 * 60 * 1000,
  });

  useEffect(() => {
    const off = onSourceStatusChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["sourceStatus"] });
      refetchSourceStatus();
    });
    return off;
  }, [queryClient, refetchSourceStatus]);

  // Загрузка сохранённых настроек при монтировании. Плавный переход и
  // нормализация гидратируются в сторе (при старте приложения), здесь грузим
  // только «высокое качество» — оно остаётся локальным состоянием экрана.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const hq = await getSetting(KEY_HIGH_QUALITY);
      if (cancelled) return;
      if (hq) setHighQuality(hq === "1");
      setLoaded(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Персист «высокого качества» при изменении (после первичной загрузки).
  // Плавный переход и нормализация персистятся самим стором в его сеттерах.
  useEffect(() => {
    if (!loaded) return;
    void setSetting(KEY_HIGH_QUALITY, highQuality ? "1" : "0");
  }, [highQuality, loaded]);

  const refreshLocal = () => {
    queryClient.invalidateQueries({ queryKey: ["localFolders"] });
    queryClient.invalidateQueries({ queryKey: ["localTracks"] });
  };

  const scanDeviceMutation = useMutation({
    mutationFn: async () => {
      const path = await scanDeviceMusic();
      if (!path) return null;
      return addLocalFolder(path);
    },
    onSuccess: refreshLocal,
  });

  const addFolderMutation = useMutation({
    mutationFn: async () => {
      const path = await pickMusicFolder();
      if (!path) return null;
      return addLocalFolder(path);
    },
    onSuccess: refreshLocal,
  });

  const removeFolderMutation = useMutation({
    mutationFn: (path: string) => removeLocalFolder(path),
    onSuccess: refreshLocal,
  });

  const validateSourceMutation = useMutation({
    mutationFn: validateSource,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["sourceStatus"] }),
  });

  return (
    <div className="flex max-w-5xl flex-col gap-6 py-2">
      <PageHeader title="Настройки" icon={Settings2} />

      {/* Вкладки: раньше всё было одним длинным свитком («большая каша»).
          Теперь разложено по смыслу — обычному пользователю достаточно вкладки
          «Музыка» (добавить папку — и всё работает), остальное по необходимости.
          Подчёркнутая полоса вкладок совпадает с макетом настроек. */}
      <TabBar tabs={SETTINGS_TABS} active={tab} onChange={setTab} />

      {/* ── Музыка ─────────────────────────────────────────────────────── */}
      {tab === "music" && (
        <div className="columns-1 gap-5 lg:columns-2">
          {/* Локальная библиотека — самый простой путь «работает из коробки». */}
          <Section title="Локальная библиотека" description="Укажите папку с музыкой — Cryon найдёт файлы, метаданные и обложки. Этого достаточно, чтобы начать слушать." anchorRef={localConfigRef}>
            <div className="flex flex-col gap-2">
              {folders.map((f) => (
                <div key={f} className="flex items-center gap-3 rounded-xl bg-white/[0.05] p-3">
                  <FolderOpen size={18} className="text-slate-400" />
                  <span className="flex-1 truncate font-mono text-sm text-slate-300">{f}</span>
                  <button
                    onClick={() => removeFolderMutation.mutate(f)}
                    className="text-sm text-slate-400 transition-colors hover:text-white"
                  >
                    Удалить
                  </button>
                </div>
              ))}
              <button
                onClick={() => scanDeviceMutation.mutate()}
                disabled={scanDeviceMutation.isPending}
                className="flex items-center justify-center gap-2 rounded-xl border border-[#a855f7]/40 bg-[#a855f7]/10 py-3 text-sm text-[#d8b4fe] transition-colors hover:bg-[#a855f7]/20 disabled:opacity-50"
              >
                <RefreshCw size={16} />
                {scanDeviceMutation.isPending ? "\u0421\u043a\u0430\u043d\u0438\u0440\u043e\u0432\u0430\u043d\u0438\u0435\u2026" : "\u0421\u043a\u0430\u043d\u0438\u0440\u043e\u0432\u0430\u0442\u044c \u043c\u0443\u0437\u044b\u043a\u0443 \u043d\u0430 \u0442\u0435\u043b\u0435\u0444\u043e\u043d\u0435"}
              </button>
              <button
                onClick={() => addFolderMutation.mutate()}
                disabled={addFolderMutation.isPending}
                className="flex items-center justify-center gap-2 rounded-xl border border-dashed border-white/15 py-3 text-sm text-slate-400 transition-colors hover:border-white/30 hover:text-white disabled:opacity-50"
              >
                <FolderOpen size={16} />
                {addFolderMutation.isPending ? "Сканирование…" : "Добавить папку"}
              </button>
            </div>
          </Section>

          {/* Обзор источников со статусом и переходом к настройке. */}
          <Section title="Источники музыки" description="Статус подключения сервисов. Кнопка ведёт к настройке нужного источника во вкладке «Подключения».">
            <div className="flex flex-col gap-2">
              {servicesLoading ? (
                <div className="rounded-xl bg-white/[0.05] p-4 text-sm text-slate-400">
                  Загрузка статуса сервисов...
                </div>
              ) : (
                services.map((s) => {
                  const meta = SOURCES[s.id];
                  const configurable = Boolean(configRefs[s.id]);
                  const actionLabel = s.id === "local"
                    ? (s.connected ? "Управлять" : "Добавить папку")
                    : !configurable
                      ? "Подробнее"
                      : s.connected
                        ? "Настроить"
                        : "Подключить";
                  return (
                    <div
                      key={s.id}
                      className="flex items-center gap-3 rounded-xl bg-white/[0.05] p-3"
                    >
                      <ServiceIcon id={s.id} size={36} badge />

                      <div className="flex-1">
                        <div className="font-medium text-white">{meta.name}</div>
                        <div className="text-xs text-slate-400">
                          {s.error ? `Ошибка проверки: ${s.error}` : s.connected ? "Готово" : "Требует настройки"}
                        </div>
                      </div>
                      {(["yandex", "spotify", "youtube"] as SourceId[]).includes(s.id) && (
                        <button
                          type="button"
                          onClick={() => validateSourceMutation.mutate(s.id)}
                          disabled={validateSourceMutation.isPending}
                          className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-white/10 hover:text-white disabled:opacity-50"
                          title="Проверить подключение"
                          aria-label={`Проверить ${meta.name}`}
                        >
                          <RefreshCw size={16} className={validateSourceMutation.isPending ? "animate-spin" : undefined} />
                        </button>
                      )}
                      <button
                        onClick={() => focusConfigSection(s.id)}
                        className={cn(
                          "rounded-lg px-4 py-1.5 text-sm font-medium transition-colors",
                          s.connected ? "bg-white/10 text-slate-300 hover:bg-white/15" : "bg-[#a855f7] text-white hover:bg-[#9333ea]",
                        )}
                      >
                        {actionLabel}
                      </button>
                    </div>
                  );
                })
              )}
            </div>
          </Section>

          <ImportFavoritesSection />
        </div>
      )}

      {/* ── Подключения ────────────────────────────────────────────────── */}
      {tab === "connections" && (
        <div className="columns-1 gap-5 lg:columns-2">
          <YandexTokenSection anchorRef={yandexConfigRef} />
          <LastFMSection anchorRef={lastfmConfigRef} />
          <SpotifySection anchorRef={spotifyConfigRef} />
          <SoundCloudSection anchorRef={soundcloudConfigRef} />
          <YouTubeSection anchorRef={youtubeConfigRef} />
          <AccountConnectionsSection />
        </div>
      )}

      {/* ── Звук ───────────────────────────────────────────────────────── */}
      {tab === "playback" && (
        <div className="columns-1 gap-5 lg:columns-2">
          <Section title="Воспроизведение" description="Поведение плеера при проигрывании музыки.">
            <div className="flex flex-col gap-1">
              <Toggle label="Плавный переход между треками" checked={crossfade} onChange={setCrossfade} />
              <Toggle label="Нормализация громкости" checked={normalizeVolume} onChange={setNormalizeVolume} />
              <Toggle label="Высокое качество звука" checked={highQuality} onChange={setHighQuality} />
            </div>
          </Section>

          <Section title="Эквалайзер" description="Тонкая настройка звучания: 5 полос и готовые пресеты. Работает и для mpv, и для встроенного плеера.">
            <button
              type="button"
              onClick={() => setEqualizerOpen(true)}
              className="flex w-full items-center justify-center gap-2 rounded-xl border border-white/10 bg-white/5 py-3 text-sm text-slate-200 transition-colors hover:border-[var(--app-accent)] hover:text-white"
            >
              <SlidersHorizontal size={17} /> Открыть эквалайзер
            </button>
          </Section>

          <Section title="Выбор версии трека" description="Эти правила помогают выбрать вариант, когда Cryon ищет воспроизводимую или запасную версию трека.">
            <div className="flex flex-col gap-1">
              <Toggle
                label="Предпочитать локальную копию"
                checked={playbackPreferences.preferLocal}
                onChange={(preferLocal) => playbackPreferencesMutation.mutate({ ...playbackPreferences, preferLocal })}
              />
            </div>
            <p className="text-xs text-slate-400">Выбор explicit и официальных записей появится после добавления надёжных признаков от источников.</p>
            <div className="mt-3 border-t border-white/8 pt-3">
              <div className="mb-2 text-sm text-slate-300">Приоритет сервисов</div>
              <div className="flex flex-col gap-1">
                {orderedPlaybackSources.map((source, index) => (
                  <div key={source} className="flex items-center gap-3 rounded-xl bg-white/[0.05] p-2">
                    <ServiceIcon id={source} size={28} badge />
                    <span className="flex-1 text-sm text-white">{SOURCES[source].name}</span>
                    <button
                      type="button"
                      onClick={() => movePlaybackSource(source, -1)}
                      disabled={index === 0 || playbackPreferencesMutation.isPending}
                      className="grid h-8 w-8 place-items-center rounded-lg text-slate-400 transition-colors hover:bg-white/10 hover:text-white disabled:opacity-30"
                      title="Поднять приоритет"
                      aria-label="Поднять приоритет"
                    >
                      <ChevronUp size={16} />
                    </button>
                    <button
                      type="button"
                      onClick={() => movePlaybackSource(source, 1)}
                      disabled={index === orderedPlaybackSources.length - 1 || playbackPreferencesMutation.isPending}
                      className="grid h-8 w-8 place-items-center rounded-lg text-slate-400 transition-colors hover:bg-white/10 hover:text-white disabled:opacity-30"
                      title="Понизить приоритет"
                      aria-label="Понизить приоритет"
                    >
                      <ChevronDown size={16} />
                    </button>
                  </div>
                ))}
              </div>
            </div>
          </Section>
        </div>
      )}

      {/* ── Диагностика ───────────────────────────────────────────────── */}
      {tab === "diagnostics" && (
        <div className="columns-1 gap-5 lg:columns-2">
          <Section
            title="Переключения и ошибки"
            description="Последние события воспроизведения на этом устройстве."
          >
            {playbackDiagnostics.length === 0 ? (
              <p className="rounded-xl bg-white/[0.05] p-3 text-sm text-slate-400">
                За последнее время проблем с воспроизведением не было.
              </p>
            ) : (
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-end gap-2">
                  <button
                    type="button"
                    onClick={exportPlaybackDiagnostics}
                    className="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-xs text-slate-300 transition-colors hover:bg-white/10 hover:text-white"
                  >
                    <Download size={16} /> Сохранить отчёт
                  </button>
                  <button
                    type="button"
                    onClick={clearPlaybackDiagnostics}
                    className="grid h-8 w-8 place-items-center rounded-lg text-slate-400 transition-colors hover:bg-white/10 hover:text-white"
                    title="Очистить историю"
                    aria-label="Очистить историю диагностики"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
                {playbackDiagnostics.map((event) => {
                  const switched = event.kind === "fallback" && event.toSource;
                  return (
                    <div key={event.id} className="rounded-xl bg-white/[0.05] p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className={switched || event.kind === "started" ? "text-sm font-medium text-emerald-300" : "text-sm font-medium text-amber-300"}>
                            {event.kind === "started" ? "Воспроизведение началось" : switched ? "Источник переключён" : "Воспроизведение остановлено"}
                          </div>
                          <div className="truncate text-sm text-white">{event.artist} — {event.title}</div>
                        </div>
                        <span className="shrink-0 text-[11px] text-slate-500">{relativeTime(event.createdAt)}</span>
                      </div>
                      <p className="mt-1 text-xs leading-relaxed text-slate-400">
                        {switched
                          ? "С " + sourceName(event.fromSource) + " на " + sourceName(event.toSource!) + " · продолжено с " + formatDuration(event.positionSeconds) + ". " + event.message
                          : event.message}
                      </p>
                      {event.fallbackSearchMs !== undefined && <p className="mt-1 text-xs text-slate-500">Поиск запасного источника: {(event.fallbackSearchMs / 1000).toFixed(1)} с</p>}
                      {event.startupMs !== undefined && <p className="mt-1 text-xs text-slate-500">Время до запуска: {(event.startupMs / 1000).toFixed(1)} с</p>}
                    </div>
                  );
                })}
              </div>
            )}
          </Section>

          <Section title="Состояние источников" description="Текущая готовность сервисов, из которых Cryon ищет и воспроизводит музыку.">
            <div className="flex flex-col gap-2">
              {services.map((service) => (
                <div key={service.id} className="flex items-center gap-3 rounded-xl bg-white/[0.05] p-3">
                  <ServiceIcon id={service.id} size={30} badge />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-white">{SOURCES[service.id].name}</div>
                    <div className={service.error ? "truncate text-xs text-amber-300" : "text-xs text-slate-400"}>
                      {service.error || (service.connected ? "Готов к работе" : "Требует настройки")}
                    </div>
                  </div>
                  <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", service.connected ? "bg-emerald-400" : "bg-slate-500")} />
                </div>
              ))}
            </div>
          </Section>
        </div>
      )}

      {/* ── Общее ──────────────────────────────────────────────────────── */}
      {tab === "general" && (
        <>
          <div className="columns-1 gap-5 lg:columns-2">
            <UpdateSection />
            <MobileServerSection />
            <Section title="Интерфейс" description="Оформление приложения.">
              <div className="flex flex-col gap-1">
                <div className="flex items-center justify-between rounded-xl bg-white/[0.05] p-3">
                  <span className="text-white">Язык интерфейса</span>
                  <span className="rounded-lg bg-white/10 px-3 py-1 text-sm text-slate-300">Русский</span>
                </div>
                <div className="flex items-center justify-between rounded-xl bg-white/[0.05] p-3">
                  <span className="text-white">Тема</span>
                  <span className="rounded-lg bg-white/10 px-3 py-1 text-sm text-slate-300">По обложке (неон)</span>
                </div>
              </div>
            </Section>

            <Section title="Помощь и обучение" description="Краткая инструкция по основным возможностям Cryon.">
              <button
                type="button"
                onClick={() => window.dispatchEvent(new Event("cryon:show-onboarding"))}
                className="flex w-full items-center justify-center gap-2 rounded-xl border border-white/10 bg-white/5 py-3 text-sm text-slate-200 transition-colors hover:border-[var(--app-accent)] hover:text-white"
              >
                <CircleHelp size={17} /> Открыть обучение заново
              </button>
            </Section>

            <ResetDataSection />
          </div>

        </>
      )}
    </div>
  );
}

/** Мы внутри нативной Android-оболочки (Capacitor)? */
function isMobileShell(): boolean {
  const cap = (window as unknown as { Capacitor?: { isNativePlatform?: () => boolean } }).Capacitor;
  return Boolean(cap?.isNativePlatform?.());
}

function UpdateSection() {
  const [state, setState] = useState<"idle" | "checking" | "downloading">("idle");
  const [message, setMessage] = useState("");
  const [release, setRelease] = useState<{ available?: boolean; latest?: string; notesUrl?: string } | null>(null);
  const android = hasNativeAndroidPlayer();

  const check = async () => {
    setState("checking");
    setMessage("");
    try {
      const response = await fetch(apiV1BaseUrl() + "/v1/system/update");
      const body = await response.json() as { available?: boolean; latest?: string; notesUrl?: string; error?: string };
      if (!response.ok) throw new Error(body.error || "Не удалось проверить обновления");
      setRelease(body);
      setMessage(body.available ? `Доступна версия ${body.latest}` : "Установлена последняя версия");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Не удалось проверить обновления");
    } finally {
      setState("idle");
    }
  };

  const install = async () => {
    setState("downloading");
    setMessage("Загружаем обновление и проверяем файл…");
    if (android && window.CryonAndroid?.installUpdate) {
      window.CryonAndroid.installUpdate();
      return;
    }
    try {
      const app = (window as unknown as { go?: { main?: { App?: { InstallUpdate?: () => Promise<void> } } }; runtime?: { Quit?: () => void } });
      if (!app.go?.main?.App?.InstallUpdate || !app.runtime?.Quit) throw new Error("Установка обновления в этой версии недоступна");
      await app.go.main.App.InstallUpdate();
      app.runtime.Quit();
    } catch (error) {
      setState("idle");
      setMessage(error instanceof Error ? error.message : "Не удалось установить обновление");
    }
  };

  useEffect(() => {
    const handler = (event: Event) => {
      const error = (event as CustomEvent<string>).detail;
      setState("idle");
      setMessage(error || "Файл готов. Подтвердите установку в системном окне Android.");
    };
    window.addEventListener("cryon:update-result", handler);
    return () => window.removeEventListener("cryon:update-result", handler);
  }, []);

  return (
    <Section title="Обновления" description="Cryon проверяет официальный GitHub-релиз и загружает подходящий файл. Ваши избранные, настройки и история сохраняются.">
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" onClick={check} disabled={state !== "idle"} className="rounded-xl bg-white/10 px-4 py-2 text-sm text-slate-200 hover:bg-white/15 disabled:opacity-50">{state === "checking" ? "Проверяем…" : "Проверить обновления"}</button>
        {release?.available && <button type="button" onClick={() => void install()} disabled={state !== "idle"} className="rounded-xl bg-[#a855f7] px-4 py-2 text-sm font-medium text-white hover:bg-[#9333ea] disabled:opacity-50">{state === "downloading" ? "Загружаем…" : "Установить"}</button>}
        {release?.notesUrl && <a href={release.notesUrl} target="_blank" rel="noreferrer" className="text-sm text-[#c084fc] hover:text-white">Открыть релиз</a>}
      </div>
      {message && <p className="mt-3 text-sm text-slate-400">{message}</p>}
      {!android && <p className="mt-3 text-xs text-slate-500">Для Windows загрузка и перезапуск выполняются автоматически после выбора установки.</p>}
    </Section>
  );
}

function relativeTime(iso: string): string {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60_000));
  if (minutes < 1) return "только что";
  if (minutes < 60) return String(minutes) + " мин назад";
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return String(hours) + " ч назад";
  return new Date(iso).toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
}

/**
 * Адрес десктоп-приложения Cryon в локальной сети (для телефона). Сохраняется в
 * localStorage; после сохранения приложение перезапускается, и HTTP-мост
 * (httpBridge) переустанавливается на новый адрес — весь остальной код работает
 * как на десктопе. На самом десктопе секция не показывается: там нативный Wails.
 */
function MobileServerSection() {
  const [url, setUrl] = useState<string>(() => configuredBaseUrl());
  const [busy, setBusy] = useState(false);
  const [devices, setDevices] = useState<Array<{ id: string; name: string; url: string; version?: string }>>([]);
  const [discovering, setDiscovering] = useState(false);
  const apply = () => {
    setConfiguredBaseUrl(url);
    setBusy(false);
  };
  const trimmed = url.trim();
  const discover = async () => {
    setDiscovering(true);
    try { const found = await discoverDevices(); setDevices(Array.isArray(found) ? found : []); } catch { setDevices([]); } finally { setDiscovering(false); }
  };
  useEffect(() => { void discover(); }, []);
  return (
    <Section
      title="Устройства Cryon в сети"
      description="Cryon автоматически ищет другие открытые приложения в вашей Wi-Fi-сети. Выберите устройство, чтобы подключить общий плеер и очередь."
    >
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => void discover()} disabled={discovering} className="flex-1 rounded-xl border border-white/10 bg-white/5 py-2.5 text-sm text-slate-200 transition-colors hover:border-[var(--app-accent)] hover:text-white disabled:opacity-60">
            {discovering ? "Поиск устройств…" : "Найти устройства автоматически"}
          </button>
        </div>
        {devices.length > 0 && <div className="flex flex-col gap-1 rounded-xl bg-white/[0.04] p-2">
          {devices.map((device) => <button key={device.id} type="button" onClick={() => setUrl(device.url)} className="flex items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-white/10">
            <span className="text-white">{device.name}</span><span className="font-mono text-xs text-slate-400">{device.url}</span>
          </button>)}
        </div>}
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          inputMode="url"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          placeholder="http://192.168.х.х:8899"
          className="rounded-xl bg-white/[0.06] px-3 py-2.5 font-mono text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/10"
        />
        <button
          type="button"
          onClick={apply}
          disabled={busy}
          className="flex items-center justify-center gap-2 rounded-xl border border-white/10 bg-white/5 py-2.5 text-sm text-slate-200 transition-colors hover:border-[var(--app-accent)] hover:text-white disabled:opacity-60"
        >
          {busy ? "Перезапуск…" : trimmed ? "Подключиться и синхронизировать" : "Отключить синхронизацию"}
        </button>
      </div>
    </Section>
  );
}

function AccountConnectionsSection() {
  const queryClient = useQueryClient();
  const { data: connections = [], isLoading } = useQuery({
    queryKey: ["accountConnections"],
    queryFn: listAccountConnections,
  });
  const [values, setValues] = useState<Record<string, { clientID: string; clientSecret: string }>>({});
  const mutation = useMutation({
    mutationFn: ({ service, clientID, clientSecret }: { service: "spotify" | "youtube" | "soundcloud"; clientID: string; clientSecret: string }) =>
      setOAuthApplicationConfig(service, clientID, clientSecret),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["accountConnections"] }),
  });
  const spotifyLoginMutation = useMutation({
    mutationFn: startSpotifyLogin,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["accountConnections"] }),
  });

  return (
    <Section
      title="Аккаунты и импорт медиатеки"
      description="Отдельно от ключей поиска настройте OAuth-приложения, чтобы далее подключать личные аккаунты и импортировать избранное или плейлисты. Секреты сохраняются только локально и никогда не показываются интерфейсу."
    >
      <div className="flex flex-col gap-3">
        {isLoading ? <p className="text-sm text-slate-400">Загрузка интеграций…</p> : connections.map((connection) => {
          const isYandex = connection.service === "yandex";
          const oauthService: "spotify" | "youtube" | "soundcloud" | null = isYandex
            ? null
            : (connection.service as "spotify" | "youtube" | "soundcloud");
          const value = values[connection.service] ?? { clientID: "", clientSecret: "" };
          const needsSecret = connection.service === "soundcloud";
          const canSave = value.clientID.trim().length > 0 && (!needsSecret || value.clientSecret.trim().length > 0);
          const canStartSpotifyLogin = connection.service === "spotify" && connection.oauthConfigured && !connection.accountConnected;
          const setValue = (field: "clientID" | "clientSecret", next: string) => {
            setValues((previous) => ({ ...previous, [connection.service]: { ...value, [field]: next } }));
          };
          return (
            <div key={connection.service} className="rounded-xl bg-white/[0.05] p-4">
              <div className="flex items-start gap-3">
                <ServiceIcon id={connection.service} size={32} badge />
                <div className="flex-1">
                  <div className="font-medium text-white">{connection.name}</div>
                  <p className="mt-1 text-xs text-slate-400">{connection.setupHint}</p>
                  <div className="mt-2 flex flex-wrap gap-2 text-xs">
                    <span className={connection.oauthConfigured ? "text-emerald-400" : "text-amber-300"}>{connection.oauthConfigured ? "OAuth-приложение настроено" : "OAuth-приложение не настроено"}</span>
                    <span className={connection.accountConnected ? "text-emerald-400" : "text-slate-500"}>{connection.accountConnected ? "Аккаунт подключён" : "Аккаунт ещё не подключён"}</span>
                  </div>
                </div>
              </div>
              {isYandex ? (
                <p className="mt-3 text-xs text-slate-400">Yandex Music использует токен из секции выше. Импорт избранного уже доступен после подключения.</p>
              ) : (
                <div className="mt-3 flex flex-col gap-2">
                  <input type="text" value={value.clientID} onChange={(event) => setValue("clientID", event.target.value)} placeholder="OAuth Client ID" className="rounded-xl bg-black/20 px-3 py-2 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]" autoComplete="off" spellCheck={false} />
                  {needsSecret && <input type="password" value={value.clientSecret} onChange={(event) => setValue("clientSecret", event.target.value)} placeholder="OAuth Client Secret" className="rounded-xl bg-black/20 px-3 py-2 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]" autoComplete="off" spellCheck={false} />}
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <span className="text-xs text-slate-500">{connection.canImportLibrary ? "Импорт доступен" : connection.service === "spotify" ? "Перед первым входом добавьте Redirect URI http://127.0.0.1:8888/callback в Spotify Dashboard." : "Вход и импорт будут доступны после запуска OAuth."}</span>
                    <div className="flex gap-2">
                      {canStartSpotifyLogin && <button type="button" onClick={() => spotifyLoginMutation.mutate()} disabled={spotifyLoginMutation.isPending} className="rounded-lg bg-emerald-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-emerald-500 disabled:opacity-40">{spotifyLoginMutation.isPending ? "Открываем браузер…" : "Войти в Spotify"}</button>}
                      <button type="button" onClick={() => oauthService && mutation.mutate({ service: oauthService, clientID: value.clientID.trim(), clientSecret: value.clientSecret.trim() })} disabled={!canSave || mutation.isPending} className="rounded-lg bg-[#a855f7] px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40">{mutation.isPending ? "Сохранение…" : "Сохранить OAuth"}</button>
                    </div>
                  </div>
                  {connection.service === "spotify" && spotifyLoginMutation.isError && <p className="text-xs text-rose-300">{spotifyLoginMutation.error instanceof Error ? spotifyLoginMutation.error.message : "Не удалось начать вход Spotify."}</p>}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </Section>
  );
}

function ImportFavoritesSection() {
  const [message, setMessage] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<FavoritesImportResult | null>(null);
  const [spotifyPreview, setSpotifyPreview] = useState<{ path?: string; file?: File; tracks: Track[] } | null>(null);
  const spotifyFileInput = useRef<HTMLInputElement | null>(null);
  const [selectedSpotifyIds, setSelectedSpotifyIds] = useState<Set<string>>(new Set());
  const [source, setSource] = useState<"yandex" | "spotify">("yandex");
  const queryClient = useQueryClient();
  const favoriteIds = useFavoriteIds();
  const mutation = useMutation({
    mutationFn: () => importFavorites(source),
    onSuccess: (result) => {
      setLastResult(result);
      setMessage(`Импорт завершён: найдено ${result.found}, добавлено ${result.imported}, уже было ${result.skipped}.`);
    },
    onError: (error) => setMessage(error instanceof Error ? error.message : "Не удалось импортировать избранное."),
  });
  const spotifyExportMutation = useMutation({
    mutationFn: async (selection: { path?: string; file?: File; trackIds: string[] }) => {
      if (selection.file) return await spotifyFileImportV1(selection.file, "import", selection.trackIds.map((id) => splitTrackId(id).rawId)) as FavoritesImportResult;
      if (!selection.path) throw new Error("Выберите файл выгрузки");
      return importSpotifyLibraryExport({ path: selection.path, trackIds: selection.trackIds });
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
      setSpotifyPreview(null);
      setLastResult(result);
      setMessage(`Импорт завершён: найдено ${result.found}, добавлено ${result.imported}, уже было ${result.skipped}.`);
    },
    onError: (error) => setMessage(error instanceof Error ? error.message : "Не удалось импортировать выгрузку Spotify."),
  });
  const spotifyPreviewMutation = useMutation({
    mutationFn: previewSpotifyLibraryExport,
    onSuccess: (result) => {
      if (result) {
        setSpotifyPreview(result);
        setSelectedSpotifyIds(new Set(result.tracks.map((track) => track.id)));
        setLastResult(null);
        setMessage(null);
      }
    },
    onError: (error) => setMessage(error instanceof Error ? error.message : "Не удалось прочитать выгрузку Spotify."),
  });
  const mobileSpotifyPreview = useMutation({
    mutationFn: async (file: File) => ({ file, tracks: mapTracks(await spotifyFileImportV1(file, "preview") as domain.Track[]) }),
    onSuccess: (result) => {
      setSpotifyPreview(result);
      setSelectedSpotifyIds(new Set(result.tracks.map((track) => track.id)));
      setLastResult(null);
      setMessage(null);
    },
    onError: (error) => setMessage(error instanceof Error ? error.message : "Не удалось прочитать выгрузку Spotify."),
  });

  return (
    <Section title="Импорт избранного" description="Перенесите любимые треки из подключённого сервиса в локальное избранное Cryon. Повторный импорт безопасен: дубликаты не создаются.">
      <div className="flex flex-col gap-3 rounded-xl bg-white/[0.05] p-4">
        <div className="flex items-center gap-3">
          <ServiceIcon id={source} size={32} badge />
          <div className="flex-1">
            <div className="font-medium text-white">{source === "yandex" ? "Yandex Music" : "Spotify"}</div>
            <p className="text-xs text-slate-400">{source === "yandex" ? "Требуется OAuth-токен из раздела выше." : "Без OAuth: скачайте архив данных Spotify и выберите его здесь. Файл остаётся на этом компьютере."}</p>
          </div>
          <button
            type="button"
            onClick={() => source === "spotify" ? (hasNativeAndroidPlayer() ? spotifyFileInput.current?.click() : spotifyPreviewMutation.mutate()) : mutation.mutate()}
            disabled={mutation.isPending || spotifyExportMutation.isPending || spotifyPreviewMutation.isPending || mobileSpotifyPreview.isPending}
            className="inline-flex items-center gap-2 rounded-lg bg-[#a855f7] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Download size={16} />
            {mutation.isPending || spotifyExportMutation.isPending ? "Импорт…" : spotifyPreviewMutation.isPending ? "Читаем файл…" : source === "spotify" ? "Выбрать выгрузку" : "Импортировать"}
          </button>
        </div>
        <input ref={spotifyFileInput} type="file" accept=".zip,.json,application/zip,application/json" className="hidden" onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ""; if (file) mobileSpotifyPreview.mutate(file); }} />
        <div className="flex gap-2">
          {(["yandex", "spotify"] as const).map((id) => <button key={id} type="button" onClick={() => { setSource(id); setMessage(null); }} disabled={mutation.isPending} className={cn("rounded-lg px-3 py-1.5 text-xs font-medium transition-colors", source === id ? "bg-white/15 text-white" : "bg-white/5 text-slate-400 hover:bg-white/10")}>{id === "yandex" ? "Yandex Music" : "Spotify"}</button>)}
        </div>
        {source === "spotify" && <p className="text-xs text-slate-500">В Spotify откройте Account → Privacy settings → Download your data. После получения архива выберите ZIP или файл YourLibrary.json.</p>}
        {source === "spotify" && spotifyPreview && (
          <div className="rounded-xl bg-black/20 p-3">
            <p className="text-sm font-medium text-white">Найдено треков: {spotifyPreview.tracks.length}</p>
            <p className="mt-1 text-xs text-slate-400">Отмечено: {selectedSpotifyIds.size}. Пока ничего не добавлено в избранное.</p>
            <div className="mt-2 flex gap-3 text-xs text-slate-300">
              <button type="button" onClick={() => setSelectedSpotifyIds(new Set(spotifyPreview.tracks.map((track) => track.id)))} className="hover:text-white">Выбрать все</button>
              <button type="button" onClick={() => setSelectedSpotifyIds(new Set())} className="hover:text-white">Снять выбор</button>
            </div>
            <ul className="mt-2 max-h-40 space-y-1 overflow-y-auto text-xs text-slate-300">
              {spotifyPreview.tracks.map((track, index) => <li key={track.id + index}><label className="flex items-center gap-2 py-1"><input type="checkbox" checked={selectedSpotifyIds.has(track.id)} onChange={(event) => setSelectedSpotifyIds((previous) => { const next = new Set(previous); if (event.target.checked) next.add(track.id); else next.delete(track.id); return next; })} /><span className="min-w-0 flex-1 truncate">{track.artist} — {track.title}</span>{favoriteIds.has(track.id) && <span className="shrink-0 text-slate-500">Уже в избранном</span>}</label></li>)}
            </ul>
            <div className="mt-3 flex gap-2">
              <button type="button" onClick={() => spotifyExportMutation.mutate({ path: spotifyPreview.path, file: spotifyPreview.file, trackIds: [...selectedSpotifyIds] })} disabled={spotifyExportMutation.isPending || selectedSpotifyIds.size === 0} className="rounded-lg bg-[#a855f7] px-3 py-2 text-sm text-white disabled:opacity-50">Добавить выбранные</button>
              <button type="button" onClick={() => setSpotifyPreview(null)} disabled={spotifyExportMutation.isPending} className="rounded-lg bg-white/10 px-3 py-2 text-sm text-slate-300 disabled:opacity-50">Отмена</button>
            </div>
          </div>
        )}
        {message && <p className={cn("text-sm", mutation.isError || spotifyExportMutation.isError || spotifyPreviewMutation.isError || mobileSpotifyPreview.isError ? "text-rose-300" : "text-emerald-300")}>{message}</p>}
        {lastResult && (
          <div className="grid grid-cols-3 gap-2 text-center">
            <div className="rounded-lg bg-black/15 p-2">
              <div className="text-lg font-semibold text-white">{lastResult.found}</div>
              <div className="text-[11px] text-slate-400">Найдено</div>
            </div>
            <div className="rounded-lg bg-black/15 p-2">
              <div className="text-lg font-semibold text-emerald-300">{lastResult.imported}</div>
              <div className="text-[11px] text-slate-400">Добавлено</div>
            </div>
            <div className="rounded-lg bg-black/15 p-2">
              <div className="text-lg font-semibold text-slate-300">{lastResult.skipped}</div>
              <div className="text-[11px] text-slate-400">Уже было</div>
            </div>
          </div>
        )}
      </div>
    </Section>
  );
}

interface SectionAnchorProps {
  anchorRef?: React.RefObject<HTMLDivElement | null>;
}

function YandexTokenSection({ anchorRef }: SectionAnchorProps) {
  const [token, setToken] = useState("");
  const [connected, setConnected] = useState(false);
  const [saving, setSaving] = useState(false);
  // После открытия браузера показываем поле для вставки токена и подсказку.
  const [awaitingPaste, setAwaitingPaste] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void yandexTokenConnected().then((v) => {
      if (!cancelled) setConnected(v);
    });
    // Токен может быть применён из другого места — обновляем статус по событию.
    const off = onYandexConnected(() => {
      setConnected(true);
      setAwaitingPaste(false);
    });
    return () => {
      cancelled = true;
      off();
    };
  }, []);

  const loginViaBrowser = async () => {
    // Официальный OAuth не даёт localhost-redirect, поэтому вход идёт через
    // страницу-помощник: открываем её и раскрываем поле для вставки токена.
    await startYandexLogin();
    setAwaitingPaste(true);
  };

  const save = async () => {
    setSaving(true);
    try {
      await setYandexToken(token.trim());
      setConnected(await yandexTokenConnected());
      setToken("");
      setAwaitingPaste(false);
    } finally {
      setSaving(false);
    }
  };

  const pasteFromClipboard = async () => {
    let value = "";
    try {
      value = await navigator.clipboard.readText();
    } catch {
      const bridge = (window as Window & { CryonAndroid?: { readClipboard?: () => string } }).CryonAndroid;
      value = bridge?.readClipboard?.() ?? "";
    }
    if (value.trim()) {
      setToken(value.trim());
      setAwaitingPaste(true);
    }
  };

  const disconnect = async () => {
    setSaving(true);
    try {
      await setYandexToken("");
      setConnected(false);
      setToken("");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section
      anchorRef={anchorRef}
      title="Yandex Music (необязательно)"
      description="Необязательно: токен включает поиск через официальное API, прямой поток и радар новинок. Для получения токена откройте веб-приложение MarshalX, войдите в аккаунт и вставьте полученный токен ниже. Без токена Yandex работает через веб-поиск, а поиск/воспроизведение/рекомендации в целом — и вовсе без него."
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-2 text-xs">
          <span
            className={cn(
              "grid h-2.5 w-2.5 place-items-center rounded-full",
              connected ? "bg-emerald-400" : "bg-white/25",
            )}
          />
          <span className={connected ? "text-emerald-400" : "text-slate-400"}>
            {connected ? "Подключено" : "Не подключено (режим веб-поиска)"}
          </span>
        </div>

        {/* Основной путь: открыть браузер и вставить выданный токен. */}
        <button
          onClick={loginViaBrowser}
          disabled={saving}
          className="flex items-center justify-center gap-2 rounded-xl bg-[#ffdb4d] px-4 py-2.5 text-sm font-semibold text-black transition-colors hover:bg-[#ffd11a] disabled:opacity-40"
        >
          {connected ? "Войти заново через браузер" : "Войти через браузер"}
        </button>

        {awaitingPaste && (
          <p className="text-xs text-slate-400">
            В браузере выберите «Получить токен в браузере», войдите в Яндекс,
            откройте страницу подтверждения и введите показанный код. Вернитесь
            на страницу токена, скопируйте его, затем вернитесь в Cryon и
            вставьте токен ниже.
          </p>
        )}

        {/* Ручной ввод токена — раскрыт после входа через браузер. */}
        <details
          open={awaitingPaste}
          className="rounded-xl bg-white/[0.05] px-3 py-2"
        >
          <summary className="cursor-pointer text-sm text-slate-300">
            Ввести токен вручную
          </summary>
          <p className="mt-2 text-xs text-slate-400">
            OAuth-токен можно получить в веб-приложении MarshalX{" "}
            <a
              href="https://ym-token.marshal.dev/"
              target="_blank"
              rel="noreferrer"
              className="text-[#c084fc] underline underline-offset-2 hover:text-[#d8b4fe]"
            >
              ym-token.marshal.dev
            </a>{" "}
            или через проект yandex-music-token / yandex-music-api (см. документацию на GitHub).
          </p>
          <div className="mt-2 flex items-center gap-2">
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={connected ? "Введите новый токен, чтобы заменить" : "Вставьте OAuth-токен"}
            className="flex-1 rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
            aria-label="OAuth-токен Yandex Music"
            autoComplete="off"
            spellCheck={false}
          />
          <button
            onClick={save}
            disabled={saving || token.trim().length === 0}
            className="rounded-lg bg-[#a855f7] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
          >
            {saving ? "Сохранение…" : "Сохранить"}
          </button>
          <button
            onClick={pasteFromClipboard}
            type="button"
            className="rounded-lg bg-white/10 px-3 py-2.5 text-xs font-medium text-slate-300 transition-colors hover:bg-white/15"
          >
            Вставить из буфера
          </button>
          {connected && (
            <button
              onClick={disconnect}
              disabled={saving}
              className="rounded-lg bg-white/10 px-4 py-2.5 text-sm font-medium text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-40"
            >
              Отключить
            </button>
          )}
          </div>
        </details>
      </div>
    </Section>
  );
}

// LastFMSection — необязательный ключ Last.fm. Рекомендации работают и без него
// (бесключевой граф Deezer + кросс-источниковый подбор); ключ Last.fm лишь
// обогащает граф (числовая близость + жанровые теги) и включает 2-хоповые
// «Открытия недели». Без ключа движок остаётся онлайн через Deezer.
function LastFMSection({ anchorRef }: SectionAnchorProps) {
  const [apiKey, setApiKey] = useState("");
  const [connected, setConnected] = useState(false);
  const [saving, setSaving] = useState(false);
  const queryClient = useQueryClient();

  useEffect(() => {
    let cancelled = false;
    void lastfmConnected().then((v) => {
      if (!cancelled) setConnected(v);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // После смены ключа кеш подборок на главной устаревает (там мог осесть
  // офлайн-результат) — сбрасываем его, чтобы рекомендации перестроились сразу.
  const invalidateReco = () => {
    queryClient.invalidateQueries({ queryKey: ["recommendations"] });
    queryClient.invalidateQueries({ queryKey: ["autoMix"] });
    queryClient.invalidateQueries({ queryKey: ["newReleases"] });
  };

  const save = async () => {
    setSaving(true);
    try {
      await setLastFMKey(apiKey.trim());
      setConnected(await lastfmConnected());
      setApiKey("");
      invalidateReco();
    } finally {
      setSaving(false);
    }
  };

  const disconnect = async () => {
    setSaving(true);
    try {
      await setLastFMKey("");
      setConnected(false);
      setApiKey("");
      invalidateReco();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section
      anchorRef={anchorRef}
      title="Last.fm (необязательно)"
      description="Рекомендации работают сразу, без ключей. Ключ Last.fm — необязательное улучшение: добавляет числовую близость артистов и жанровые теги, а также включает 2-хоповые «Открытия недели». Без него граф похожести берётся из Deezer (бесключево)."
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-2 text-xs">
          <span
            className={cn(
              "grid h-2.5 w-2.5 place-items-center rounded-full",
              connected ? "bg-emerald-400" : "bg-white/25",
            )}
          />
          <span className={connected ? "text-emerald-400" : "text-slate-400"}>
            {connected ? "Подключено" : "Не подключено (только офлайн-подборки)"}
          </span>
        </div>

        <p className="text-xs text-slate-400">
          Бесплатный API-ключ выдаётся на{" "}
          <a
            href="https://www.last.fm/api/account/create"
            target="_blank"
            rel="noreferrer"
            className="text-[#c084fc] underline underline-offset-2 hover:text-[#d8b4fe]"
          >
            last.fm/api/account/create
          </a>
          . Скопируйте значение API key и вставьте его ниже.
        </p>

        <div className="flex items-center gap-2">
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder={connected ? "Введите новый ключ, чтобы заменить" : "Вставьте API key Last.fm"}
            className="flex-1 rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
            aria-label="API-ключ Last.fm"
            autoComplete="off"
            spellCheck={false}
          />
          <button
            onClick={save}
            disabled={saving || apiKey.trim().length === 0}
            className="rounded-lg bg-[#a855f7] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
          >
            {saving ? "Сохранение…" : "Сохранить"}
          </button>
          {connected && (
            <button
              onClick={disconnect}
              disabled={saving}
              className="rounded-lg bg-white/10 px-4 py-2.5 text-sm font-medium text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-40"
            >
              Отключить
            </button>
          )}
        </div>
      </div>
    </Section>
  );
}

// ConnectedBadge — унифицированный индикатор «подключено/нет» для секций
// учётных данных. accentWhenOff помечает случаи, когда без ключа источник всё
// равно работает (жёлтый), а не выключен (серый).
function ConnectedBadge({
  connected,
  onText,
  offText,
}: {
  connected: boolean;
  onText: string;
  offText: string;
}) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span
        className={cn(
          "grid h-2.5 w-2.5 place-items-center rounded-full",
          connected ? "bg-emerald-400" : "bg-white/25",
        )}
      />
      <span className={connected ? "text-emerald-400" : "text-slate-400"}>
        {connected ? onText : offText}
      </span>
    </div>
  );
}

// SpotifySection — ввод client_id/secret Spotify. С ключами доступен
// официальный Web API (поиск с метаданными и «Радар новинок»); без них Spotify
// работает через веб-поиск с внешними ссылками.
function SpotifySection({ anchorRef }: SectionAnchorProps) {
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [connected, setConnected] = useState(false);
  const [saving, setSaving] = useState(false);
  const queryClient = useQueryClient();

  useEffect(() => {
    let cancelled = false;
    void spotifyConnected().then((v) => {
      if (!cancelled) setConnected(v);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Подключение Spotify включает «Радар новинок» — сбрасываем кэш подборок.
  const invalidateReco = () => {
    queryClient.invalidateQueries({ queryKey: ["newReleases"] });
    queryClient.invalidateQueries({ queryKey: ["sourceStatus"] });
  };

  const save = async () => {
    setSaving(true);
    try {
      await setSpotifyCredentials(clientId.trim(), clientSecret.trim());
      setConnected(await spotifyConnected());
      setClientId("");
      setClientSecret("");
      invalidateReco();
    } finally {
      setSaving(false);
    }
  };

  const disconnect = async () => {
    setSaving(true);
    try {
      await setSpotifyCredentials("", "");
      setConnected(false);
      setClientId("");
      setClientSecret("");
      invalidateReco();
    } finally {
      setSaving(false);
    }
  };

  const canSave = clientId.trim().length > 0 && clientSecret.trim().length > 0;

  return (
    <Section
      anchorRef={anchorRef}
      title="Spotify"
      description="Ключи client_id и client_secret включают поиск через официальное Web API и «Радар новинок». Треки Spotify защищены DRM, поэтому играются во внешнем приложении. Без ключей Spotify работает через веб-поиск."
    >
      <div className="flex flex-col gap-3">
        <ConnectedBadge
          connected={connected}
          onText="Подключено (официальное API)"
          offText="Не подключено (веб-поиск)"
        />

        <p className="text-xs text-slate-400">
          Создайте приложение в{" "}
          <a
            href="https://developer.spotify.com/dashboard"
            target="_blank"
            rel="noreferrer"
            className="text-[#c084fc] underline underline-offset-2 hover:text-[#d8b4fe]"
          >
            панели разработчика Spotify
          </a>
          , затем скопируйте Client ID и Client secret ниже.
        </p>

        <div className="flex flex-col gap-2">
          <input
            type="text"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            placeholder={connected ? "Введите новый Client ID, чтобы заменить" : "Client ID"}
            className="rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
            aria-label="Client ID Spotify"
            autoComplete="off"
            spellCheck={false}
          />
          <div className="flex items-center gap-2">
            <input
              type="password"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              placeholder={connected ? "Введите новый Client secret" : "Client secret"}
              className="flex-1 rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
              aria-label="Client secret Spotify"
              autoComplete="off"
              spellCheck={false}
            />
            <button
              onClick={save}
              disabled={saving || !canSave}
              className="rounded-lg bg-[#a855f7] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
            >
              {saving ? "Сохранение…" : "Сохранить"}
            </button>
            {connected && (
              <button
                onClick={disconnect}
                disabled={saving}
                className="rounded-lg bg-white/10 px-4 py-2.5 text-sm font-medium text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-40"
              >
                Отключить
              </button>
            )}
          </div>
        </div>
      </div>
    </Section>
  );
}

// SoundCloudSection — ручной ввод client_id SoundCloud. Необязателен: client_id
// определяется автоматически со страниц сервиса, — но заданный вручную надёжнее.
function SoundCloudSection({ anchorRef }: SectionAnchorProps) {
  const [clientId, setClientId] = useState("");
  const [connected, setConnected] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void soundcloudConnected().then((v) => {
      if (!cancelled) setConnected(v);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await setSoundCloudClientID(clientId.trim());
      setConnected(await soundcloudConnected());
      setClientId("");
    } finally {
      setSaving(false);
    }
  };

  const disconnect = async () => {
    setSaving(true);
    try {
      await setSoundCloudClientID("");
      setConnected(false);
      setClientId("");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section
      anchorRef={anchorRef}
      title="SoundCloud"
      description="client_id необязателен: SoundCloud работает и без него — ключ определяется автоматически. Заданный вручную client_id надёжнее, если автоопределение перестало срабатывать."
    >
      <div className="flex flex-col gap-3">
        <ConnectedBadge
          connected={connected}
          onText="Задан вручную"
          offText="Автоопределение (работает без ключа)"
        />

        <p className="text-xs text-slate-400">
          client_id можно найти в запросах на{" "}
          <a
            href="https://soundcloud.com/"
            target="_blank"
            rel="noreferrer"
            className="text-[#c084fc] underline underline-offset-2 hover:text-[#d8b4fe]"
          >
            soundcloud.com
          </a>{" "}
          через инструменты разработчика (параметр client_id в URL к api-v2).
        </p>

        <div className="flex items-center gap-2">
          <input
            type="password"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            placeholder={connected ? "Введите новый client_id, чтобы заменить" : "Вставьте client_id"}
            className="flex-1 rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
            aria-label="client_id SoundCloud"
            autoComplete="off"
            spellCheck={false}
          />
          <button
            onClick={save}
            disabled={saving || clientId.trim().length === 0}
            className="rounded-lg bg-[#a855f7] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
          >
            {saving ? "Сохранение…" : "Сохранить"}
          </button>
          {connected && (
            <button
              onClick={disconnect}
              disabled={saving}
              className="rounded-lg bg-white/10 px-4 py-2.5 text-sm font-medium text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-40"
            >
              Сбросить
            </button>
          )}
        </div>
      </div>
    </Section>
  );
}

// YouTubeSection — необязательный ключ YouTube Data API. Поиск работает и без
// него (через Bing и парсинг страниц), с ключом выдача стабильнее.
function YouTubeSection({ anchorRef }: SectionAnchorProps) {
  const [apiKey, setApiKey] = useState("");
  const [connected, setConnected] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void youtubeConnected().then((v) => {
      if (!cancelled) setConnected(v);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await setYouTubeAPIKey(apiKey.trim());
      setConnected(await youtubeConnected());
      setApiKey("");
    } finally {
      setSaving(false);
    }
  };

  const disconnect = async () => {
    setSaving(true);
    try {
      await setYouTubeAPIKey("");
      setConnected(false);
      setApiKey("");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section
      anchorRef={anchorRef}
      title="YouTube"
      description="API-ключ необязателен: YouTube работает без него — поиск идёт через веб. Ключ YouTube Data API делает выдачу стабильнее при частых запросах."
    >
      <div className="flex flex-col gap-3">
        <ConnectedBadge
          connected={connected}
          onText="Ключ задан"
          offText="Без ключа (работает через веб-поиск)"
        />

        <p className="text-xs text-slate-400">
          Ключ создаётся в{" "}
          <a
            href="https://console.cloud.google.com/apis/credentials"
            target="_blank"
            rel="noreferrer"
            className="text-[#c084fc] underline underline-offset-2 hover:text-[#d8b4fe]"
          >
            Google Cloud Console
          </a>{" "}
          после включения YouTube Data API v3.
        </p>

        <div className="flex items-center gap-2">
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder={connected ? "Введите новый ключ, чтобы заменить" : "Вставьте API-ключ YouTube Data"}
            className="flex-1 rounded-xl bg-white/[0.05] px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-500 focus:bg-white/[0.09]"
            aria-label="API-ключ YouTube Data"
            autoComplete="off"
            spellCheck={false}
          />
          <button
            onClick={save}
            disabled={saving || apiKey.trim().length === 0}
            className="rounded-lg bg-[#a855f7] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#9333ea] disabled:opacity-40"
          >
            {saving ? "Сохранение…" : "Сохранить"}
          </button>
          {connected && (
            <button
              onClick={disconnect}
              disabled={saving}
              className="rounded-lg bg-white/10 px-4 py-2.5 text-sm font-medium text-slate-300 transition-colors hover:bg-white/15 disabled:opacity-40"
            >
              Сбросить
            </button>
          )}
        </div>
      </div>
    </Section>
  );
}

// ResetDataSection — полный сброс пользовательских данных к «чистому аккаунту».
// Удаляет историю, избранное, плейлисты, локальную библиотеку, настройки,
// токены источников и кэш рекомендаций. Полезно перед передачей приложения
// другому пользователю. NOTE: добавлено koda.
function ResetDataSection() {
  const [confirming, setConfirming] = useState(false);
  const [done, setDone] = useState(false);
  const queryClient = useQueryClient();

  const resetMutation = useMutation({
    mutationFn: resetAllData,
    onSuccess: () => {
      // Инвалидируем все запросы, чтобы экраны перерисовались пустыми.
      queryClient.invalidateQueries();
      setConfirming(false);
      setDone(true);
      setTimeout(() => setDone(false), 4000);
    },
  });

  return (
    <Section
      title="Сброс данных"
      description="Полностью очищает историю, избранное, плейлисты, локальную библиотеку, настройки и токены. Приложение вернётся к состоянию «чистого аккаунта». Действие необратимо."
    >
      <div className="flex flex-col gap-3">
        {done && (
          <div className="rounded-xl border border-emerald-400/30 bg-emerald-400/10 px-3 py-2 text-sm text-emerald-300">
            Все данные очищены. Приложение сброшено к чистому состоянию.
          </div>
        )}
        {!confirming ? (
          <button
            onClick={() => setConfirming(true)}
            className="flex w-fit items-center gap-2 rounded-xl border border-red-400/30 bg-red-400/10 px-4 py-2.5 text-sm font-medium text-red-300 transition-colors hover:bg-red-400/20"
          >
            Сбросить все данные
          </button>
        ) : (
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-red-400/30 bg-red-400/10 px-4 py-3">
            <span className="text-sm text-red-200">
              Точно удалить все данные? Это необратимо.
            </span>
            <button
              onClick={() => resetMutation.mutate()}
              disabled={resetMutation.isPending}
              className="rounded-lg bg-red-500 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-red-600 disabled:opacity-50"
            >
              {resetMutation.isPending ? "Сброс…" : "Да, удалить всё"}
            </button>
            <button
              onClick={() => setConfirming(false)}
              disabled={resetMutation.isPending}
              className="rounded-lg bg-white/10 px-4 py-2 text-sm font-medium text-slate-200 transition-colors hover:bg-white/15 disabled:opacity-50"
            >
              Отмена
            </button>
          </div>
        )}
      </div>
    </Section>
  );
}

function Section({
  title,
  description,
  children,
  anchorRef,
}: {
  title: string;
  description: string;
  children: ReactNode;
  anchorRef?: React.RefObject<HTMLDivElement | null>;
}) {
  return (
    <section
      ref={anchorRef}
      className="mb-5 flex break-inside-avoid flex-col gap-3 rounded-2xl border border-white/8 bg-white/[0.03] p-5"
    >
      <div>
        <h2 className="text-lg font-semibold text-white">{title}</h2>
        <p className="text-sm text-slate-400">{description}</p>
      </div>
      {children}
    </section>
  );
}

function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <button
      onClick={() => onChange(!checked)}
      className="flex items-center justify-between rounded-xl bg-white/[0.05] p-3 text-left"
    >
      <span className="text-white">{label}</span>
      <span
        className={cn(
          "grid h-6 w-11 items-center rounded-full p-0.5 transition-colors",
          checked ? "bg-[#a855f7]" : "bg-white/15",
        )}
      >
        <span
          className={cn(
            "grid h-5 w-5 place-items-center rounded-full bg-white transition-transform",
            checked ? "translate-x-5" : "translate-x-0",
          )}
        >
          {checked && <Check size={12} className="text-[#a855f7]" />}
        </span>
      </span>
    </button>
  );
}
