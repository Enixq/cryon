// Единая точка доступа фронтенда к backend через Wails-биндинги.
// В браузере (vite dev без Wails) биндингов нет — тогда используется
// мок-фолбэк, чтобы UI оставался рабочим при обычной веб-разработке.

import type { SourceId, Track } from "../types";
import type { ArtistPage } from "../types";
import { mapArtistInfo, mapTrack, mapTracks, splitTrackId, toBackendTrack } from "./mappers";
import * as App from "../../../wailsjs/go/main/App";
import { tracks as mockTracks } from "../../mocks/data";

export interface LocalAccount { id: string; login: string; email: string; isGuest: boolean; }

// Состояние web fallback нужно не только для витрины: оно позволяет проверять
// пользовательские сценарии (включая плейлисты) без запущенного Wails backend.
// В desktop-режиме единственным источником истины остаётся SQLite.
let mockUserPlaylists: UserPlaylistDto[] = [];
const mockPlaylistTracks = new Map<string, Track[]>();
let mockPlaylistSequence = 0;
let mockHistory: HistoryEntryDto[] = [];
const mockHistoryStorageKey = "cryon.mock.history";

/**
 * Фолбэк истории должен переживать смену маршрута и загрузку отдельного
 * frontend-чанка. Держим оперативную копию для скорости, а sessionStorage —
 * как общий источник состояния в браузерной версии приложения.
 */
function readMockHistory(): HistoryEntryDto[] {
  if (typeof window === "undefined") return mockHistory;
  try {
    const saved = window.sessionStorage.getItem(mockHistoryStorageKey);
    if (!saved) return mockHistory;
    const entries = JSON.parse(saved) as HistoryEntryDto[];
    return Array.isArray(entries) ? entries : mockHistory;
  } catch {
    return mockHistory;
  }
}

function saveMockHistory(entries: HistoryEntryDto[]): void {
  mockHistory = entries;
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(mockHistoryStorageKey, JSON.stringify(entries));
  } catch {
    // Недоступное storage не должно ломать воспроизведение в web fallback.
  }
}

export async function currentAccount(): Promise<LocalAccount | null> {
  if (!isWailsRuntime()) return { id: "dev", login: "Разработчик", email: "", isGuest: true };
  const result = await App.CurrentAccount();
  return result ? { id: result.id, login: result.login, email: result.email, isGuest: result.isGuest } : null;
}
export async function registerAccount(login: string, email: string, password: string): Promise<LocalAccount> { return App.RegisterAccount(login, email, password) as Promise<LocalAccount>; }
export async function loginAccount(login: string, password: string): Promise<LocalAccount> { return App.LoginAccount(login, password) as Promise<LocalAccount>; }
export async function continueAsGuest(): Promise<LocalAccount> { return App.ContinueAsGuest() as Promise<LocalAccount>; }
export async function logoutAccount(): Promise<void> { if (isWailsRuntime()) await App.LogoutAccount(); }

/** Запущены ли мы внутри Wails (есть мост window.go). */
export function isWailsRuntime(): boolean {
  return typeof window !== "undefined" && Boolean((window as Window & { go?: { main?: { App?: unknown } } }).go?.main?.App);
}

// --- Мок-фолбэк для веб-разработки ---

function mockSearch(query: string): Track[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  return mockTracks.filter(
    (t) => t.title.toLowerCase().includes(q) || t.artist.toLowerCase().includes(q),
  );
}

// --- Публичное API ---

/** Поиск по одному источнику. */
export async function search(source: string, query: string): Promise<Track[]> {
  if (!isWailsRuntime()) {
    return mockSearch(query).filter((t) => t.source === source);
  }
  const result = await App.Search(source, query);
  return mapTracks(result);
}

/** Поиск по всем источникам сразу. */
export async function searchAll(query: string): Promise<Track[]> {
  if (!isWailsRuntime()) {
    return mockSearch(query);
  }
  const result = await App.SearchAll(query);
  return mapTracks(result);
}

/** Поиск только в выбранных источниках; пустой список означает «везде». */
export async function searchInSources(query: string, sources: SourceId[]): Promise<Track[]> {
  if (!isWailsRuntime()) {
    return mockSearch(query).filter((track) => sources.length === 0 || sources.includes(track.source));
  }
  return mapTracks(await App.SearchInSources(query, sources));
}

/**
 * Страница исполнителя: фото, популярные треки и релизы (альбомы/синглы).
 * Backend агрегирует структурированный каталог (Yandex с токеном) с
 * мультипоиском по рабочим источникам. В вебе без Wails — эвристика из моков.
 */
export async function getArtist(name: string): Promise<ArtistPage> {
  const trimmed = name.trim();
  if (!isWailsRuntime()) {
    const tracks = mockSearch(trimmed).filter((t) =>
      t.artist.toLowerCase().includes(trimmed.toLowerCase()),
    );
    return {
      name: trimmed,
      coverUrl: tracks.find((t) => t.coverUrl)?.coverUrl,
      topTracks: tracks,
      albums: [],
      singles: [],
      appearsOn: [],
    };
  }
  return mapArtistInfo(await App.GetArtist(trimmed));
}

/** Трек-лист релиза по (источник, id релиза). Пусто для источников без каталога. */
export async function getAlbumTracks(service: SourceId, albumId: string): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  return mapTracks(await App.GetAlbumTracks(service, albumId));
}

/**
 * Добирает полный трек-лист альбома по «исполнитель + название». Нужен для
 * альбомов, открытых со страницы поиска: там альбом собран из отдельных
 * найденных треков и не имеет id каталога. Backend спрашивает Yandex (по
 * токену) и YouTube Music (через yt-dlp) и возвращает первый непустой список.
 * Пусто — «каталог не нашёл», страница честно оставит найденные поиском треки.
 */
export async function resolveAlbum(artist: string, title: string): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ResolveAlbum(artist, title));
  } catch {
    return [];
  }
}

/** Список подключённых источников. */
export async function listSources(): Promise<string[]> {
  if (!isWailsRuntime()) {
    return ["youtube", "soundcloud", "spotify", "yandex", "vk", "local"];
  }
  return App.ListSources();
}

/** Прямой аудиопоток трека. */
export async function getAudioStreamUrl(compositeId: string): Promise<string | null> {
  if (!isWailsRuntime()) return null;
  const { source, rawId } = splitTrackId(compositeId);
  const stream = await App.GetAudioStream(source, rawId);
  return stream?.url || null;
}

/**
 * Подобрать прямую версию той же композиции в доступных источниках. Backend
 * использует строгую проверку «исполнитель + название» и порядок SoundCloud →
 * Yandex → local → YouTube; исходный трек остаётся безопасным fallback.
 */
export async function resolvePlayableTrack(track: Track): Promise<Track> {
  if (!isWailsRuntime()) return track;
  try {
    return mapTrack(await App.ResolvePlayableTrack(toBackendTrack(track)));
  } catch {
    return track;
  }
}

/**
 * URL трека для HTML5 <audio> (фолбэк без mpv). Всё идёт через внутренний
 * ассет-сервер приложения: локальные файлы по /local/<id> (WebView2 не играет
 * абсолютные file://-пути), остальные источники — по /stream/<source>/<id>.
 *
 * Прямую ссылку внешнего сервиса здесь отдавать нельзя: googlevideo и часть CDN
 * отвечают вебвью 403 (тот же URL из процесса приложения даёт 206), и трек
 * просто не начинался. Плюс кросс-доменный источник ломает Web Audio-граф
 * эквалайзера — createMediaElementSource на нём выдаёт тишину. Через прокси
 * поток становится same-origin, с поддержкой Range (перемотка).
 */
export async function getPlaybackUrlForHtml5(compositeId: string): Promise<string | null> {
  if (!isWailsRuntime()) return null;
  // В Android/LAN-режиме base — абсолютный адрес сервера (http://IP:8899); на
  // десктопе он пуст → остаются относительные same-origin пути, как и раньше.
  const base = (window as unknown as { __CRYON_BASE__?: string }).__CRYON_BASE__ ?? "";
  const { source, rawId } = splitTrackId(compositeId);
  if (source === "local") {
    return base + "/local/" + encodeURIComponent(rawId);
  }
  return base + "/stream/" + encodeURIComponent(source) + "/" + encodeURIComponent(rawId);
}

/** Избранное: список. */
export async function listFavorites(): Promise<Track[]> {
  if (!isWailsRuntime()) {
    return mockTracks.filter((t) => t.liked).map((track) => ({ ...track, liked: true }));
  }
  const result = await App.ListFavorites();
  return mapTracks(result).map((track) => ({ ...track, liked: true }));
}

export interface FavoritesImportResult {
  source: string;
  found: number;
  imported: number;
}

/** Импортировать любимые треки подключённого внешнего сервиса в Cryon. */
export async function importFavorites(source: SourceId): Promise<FavoritesImportResult> {
  if (!isWailsRuntime()) {
    throw new Error("Импорт доступен только в приложении Cryon");
  }
  return App.ImportFavorites(source);
}

/** Импорт liked songs из официальной локальной выгрузки Spotify, без OAuth. */
export async function importSpotifyLibraryExport(): Promise<FavoritesImportResult | null> {
  if (!isWailsRuntime()) return null;
  const path = await App.PickSpotifyLibraryExport();
  return path ? await App.ImportSpotifyLibraryExport(path) : null;
}

/** Открыть браузер и начать OAuth-вход Spotify (Authorization Code + PKCE). */
export async function startSpotifyLogin(): Promise<void> {
  if (!isWailsRuntime()) {
    throw new Error("Вход Spotify доступен только в приложении Cryon");
  }
  await App.StartSpotifyLogin();
}

/**
 * Рекомендации на основе истории и избранного (похожие артисты через Last.fm
 * при наличии ключа, иначе оффлайн-фолбэк по любимым артистам). В вебе без
 * Wails возвращается пусто — HomePage тогда использует клиентский derive.
 */
export async function listRecommendations(limit = 20): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ListRecommendations(limit));
  } catch {
    return [];
  }
}

/** Доступен ли онлайн-движок рекомендаций (задан ли ключ Last.fm). */
export async function recommendationsAvailable(): Promise<boolean> {
  if (!isWailsRuntime()) return false;
  try {
    return await App.RecommendationsAvailable();
  } catch {
    return false;
  }
}

/** Задан ли ключ Last.fm (без раскрытия самого ключа). */
export async function lastfmConnected(): Promise<boolean> {
  if (!isWailsRuntime()) return false;
  try {
    return await App.LastFMConnected();
  } catch {
    return false;
  }
}

/**
 * Сохранить ключ Last.fm и сразу применить его к движку рекомендаций. Пустая
 * строка отключает онлайн-режим. Бесплатный ключ: last.fm/api/account/create.
 */
export async function setLastFMKey(apiKey: string): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.SetLastFMKey(apiKey);
}

/**
 * Оценки рекомендаций: карта «исполнитель|название» → 1 (нравится) / -1 (не
 * нравится). Ключ строится ровно так же, как на backend (trim + нижний регистр),
 * иначе подсветка кнопок не совпадёт с сохранённой оценкой.
 */
export type RecoFeedbackMap = Record<string, number>;

export function recoFeedbackKey(artist: string, title: string): string {
  return `${artist.trim()}|${title.trim()}`.toLowerCase();
}

/** Ключ localStorage для веб-режима без Wails. */
const mockFeedbackKey = "cryon.reco.feedback";

function readMockFeedback(): RecoFeedbackMap {
  try {
    const raw = localStorage.getItem(mockFeedbackKey);
    return raw ? (JSON.parse(raw) as RecoFeedbackMap) : {};
  } catch {
    return {};
  }
}

/** Сохранённые оценки рекомендаций (для подсветки кнопок на карточках). */
export async function recoFeedbackState(): Promise<RecoFeedbackMap> {
  if (!isWailsRuntime()) return readMockFeedback();
  try {
    return (await App.RecoFeedbackState()) ?? {};
  } catch {
    return {};
  }
}

/**
 * Оценить рекомендацию: score = 1 («нравится»), -1 («не нравится»), 0 — снять
 * оценку. Возвращает актуальную карту оценок, поэтому UI не делает второй
 * запрос. Оценка меняет профиль вкусов: плюс усиливает исполнителя, накопленный
 * минус исключает его из подборок, оценённый трек больше не предлагается.
 */
export async function setRecoFeedback(
  artist: string,
  title: string,
  score: number,
): Promise<RecoFeedbackMap> {
  if (!isWailsRuntime()) {
    const map = readMockFeedback();
    const key = recoFeedbackKey(artist, title);
    if (score === 0) delete map[key];
    else map[key] = score;
    try {
      localStorage.setItem(mockFeedbackKey, JSON.stringify(map));
    } catch {
      /* приватный режим браузера — оценки просто не сохранятся */
    }
    return map;
  }
  try {
    return (await App.SetRecoFeedback(artist, title, score)) ?? {};
  } catch {
    return await recoFeedbackState();
  }
}

/**
 * Треки, похожие на seed — основа умной очереди (радио). Когда очередь
 * заканчивается и включён режим радио, плеер дозаполняет её этими треками.
 * В вебе без Wails возвращает пусто.
 */
export async function listRelatedTracks(seed: Track, limit = 15): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ListRelatedTracks(toBackendTrack(seed), limit));
  } catch {
    return [];
  }
}

/** Автоподборка «Микс дня»: стабильна в течение суток, обновляется каждый день. */
export async function listDailyMix(limit = 20): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ListDailyMix(limit));
  } catch {
    return [];
  }
}

/** Автоподборка «Открытия недели»: стабильна в течение недели, обновляется раз в неделю. */
export async function listWeeklyDiscoveries(limit = 20): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ListWeeklyDiscoveries(limit));
  } catch {
    return [];
  }
}

/**
 * «Радар новинок»: свежие релизы из подключённых сервисов (Yandex с токеном,
 * Spotify с ключами). В вебе без Wails или без подключённых сервисов —
 * пустой список.
 */
export async function listNewReleases(limit = 40): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  try {
    return mapTracks(await App.ListNewReleases(limit));
  } catch {
    return [];
  }
}

/** Избранное: удаление по составному id. */
export async function removeFavorite(compositeId: string): Promise<void> {
  if (!isWailsRuntime()) return;
  const { source, rawId } = splitTrackId(compositeId);
  await App.RemoveFavorite(source, rawId);
}

/** Избранное: добавление. */
export async function addFavorite(track: Track): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.AddFavorite(toBackendTrack(track));
}

/**
 * Открывает ссылку во внешнем браузере/приложении (для треков external_only,
 * например Spotify). В вебе — обычный window.open.
 */
export async function openExternal(url: string): Promise<void> {
  if (!url) return;
  if (!isWailsRuntime()) {
    window.open(url, "_blank", "noopener,noreferrer");
    return;
  }
  const { BrowserOpenURL } = await import("../../../wailsjs/runtime/runtime");
  BrowserOpenURL(url);
}

// --- Локальная библиотека ---

/** Список треков локальной библиотеки. */
export async function listLocalTracks(): Promise<Track[]> {
  if (!isWailsRuntime()) {
    return mockTracks.filter((t) => t.source === "local");
  }
  return mapTracks(await App.ListLocalTracks());
}

/** Папки-источники локальной музыки. */
export async function listLocalFolders(): Promise<string[]> {
  if (!isWailsRuntime()) return [];
  return App.ListLocalFolders();
}

/** Открыть системный диалог выбора папки. Пусто — отменено. */
export async function pickMusicFolder(): Promise<string> {
  // Нативная Android-оболочка сама открывает SAF-диалог (Kotlin), копирует
  // выбранную папку в filesDir и возвращает путь через глобальный колбэк
  // window.__cryonFolderPickerResolve. Без этого на Android кнопка была
  // «мертва»: httpBridge глушит любые App.Pick* в "" (нет нативных диалогов у
  // встроенного сервера), поэтому App.PickMusicFolder() всегда отдавал пустую
  // строку.
  const android = (window as unknown as { CryonAndroid?: { pickMusicFolder?: () => void } }).CryonAndroid;
  if (android && typeof android.pickMusicFolder === "function") {
    return new Promise<string>((resolve) => {
      const w = window as unknown as { __cryonFolderPickerResolve?: (path: string) => void };
      let settled = false;
      w.__cryonFolderPickerResolve = (path: string) => {
        if (settled) return;
        settled = true;
        delete w.__cryonFolderPickerResolve;
        resolve(typeof path === "string" ? path : "");
      };
      try {
        android.pickMusicFolder!();
      } catch {
        // Мост недоступен — не держим промис висящим.
        if (!settled) {
          settled = true;
          delete w.__cryonFolderPickerResolve;
          resolve("");
        }
      }
    });
  }
  if (!isWailsRuntime()) return "";
  return App.PickMusicFolder();
}

/** Добавить папку и пересканировать. Возвращает актуальную библиотеку. */
export async function addLocalFolder(path: string): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  return mapTracks(await App.AddLocalFolder(path));
}

/** Убрать папку и пересканировать. */
export async function removeLocalFolder(path: string): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  return mapTracks(await App.RemoveLocalFolder(path));
}

/** Пересканировать все папки. */
export async function rescanLocalLibrary(): Promise<Track[]> {
  if (!isWailsRuntime()) return [];
  return mapTracks(await App.RescanLocalLibrary());
}

/** Обложка локального трека как data-URL (пусто, если нет). */
export async function getLocalCover(compositeId: string): Promise<string> {
  if (!isWailsRuntime()) return "";
  const { rawId } = splitTrackId(compositeId);
  try {
    return await App.GetLocalCover(rawId);
  } catch {
    return "";
  }
}

// --- Настройки (персист в SQLite) ---

/** Прочитать значение настройки. Пусто, если ключа нет или мы в вебе. */
export async function getSetting(key: string): Promise<string> {
  if (!isWailsRuntime()) {
    return localStorage.getItem(`cryon.setting.${key}`) ?? "";
  }
  try {
    return await App.GetSetting(key);
  } catch {
    return "";
  }
}

/** Сохранить значение настройки. */
export async function setSetting(key: string, value: string): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem(`cryon.setting.${key}`, value);
    return;
  }
  await App.SetSetting(key, value);
}

// --- Yandex Music: токен ---

/** Сохранить и применить OAuth-токен Yandex Music. */
export async function setYandexToken(token: string): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem("cryon.yandex.token", token);
    return;
  }
  await App.SetYandexToken(token);
}

/** Подключён ли Yandex Music по токену (сам токен не раскрывается). */
export async function yandexTokenConnected(): Promise<boolean> {
  if (!isWailsRuntime()) {
    return Boolean(localStorage.getItem("cryon.yandex.token"));
  }
  try {
    return await App.YandexTokenConnected();
  } catch {
    return false;
  }
}

/**
 * Запустить браузерный вход в Yandex Music. Открывает системный браузер на
 * странице авторизации; токен возвращается автоматически. Об успехе сообщает
 * событие Wails "yandex:connected" (см. onYandexConnected).
 */
export async function startYandexLogin(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.StartYandexLogin();
}

// --- Внешние аккаунты и OAuth-приложения ---

export type OAuthService = "spotify" | "youtube" | "soundcloud";

export interface AccountConnectionDto {
  service: "yandex" | OAuthService;
  name: string;
  oauthConfigured: boolean;
  accountConnected: boolean;
  canImportLibrary: boolean;
  setupHint: string;
}

/** Статусы OAuth-конфигурации и пользовательских аккаунтов без раскрытия секретов. */
export async function listAccountConnections(): Promise<AccountConnectionDto[]> {
  if (!isWailsRuntime()) {
    return [
      { service: "yandex", name: "Yandex Music", oauthConfigured: true, accountConnected: false, canImportLibrary: false, setupHint: "Вставьте OAuth-токен в разделе Yandex Music." },
      { service: "spotify", name: "Spotify", oauthConfigured: false, accountConnected: false, canImportLibrary: false, setupHint: "Укажите Client ID OAuth-приложения." },
      { service: "youtube", name: "YouTube Music", oauthConfigured: false, accountConnected: false, canImportLibrary: false, setupHint: "Укажите Client ID типа Desktop app." },
      { service: "soundcloud", name: "SoundCloud", oauthConfigured: false, accountConnected: false, canImportLibrary: false, setupHint: "Для личной медиатеки требуется одобренное OAuth-приложение." },
    ];
  }
  return App.ListAccountConnections() as Promise<AccountConnectionDto[]>;
}

/** Сохранить реквизиты собственного OAuth-приложения. Пользовательские токены сюда не входят. */
export async function setOAuthApplicationConfig(
  service: OAuthService,
  clientID: string,
  clientSecret = "",
): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem(`cryon.oauth.${service}.clientID`, clientID);
    return;
  }
  await App.SetOAuthApplicationConfig(service, clientID, clientSecret);
}

// --- Spotify / SoundCloud / YouTube: учётные данные ---

/**
 * Сохранить и применить client_id/secret Spotify. С ними поиск идёт через
 * официальный Web API и доступен «Радар новинок»; пустые значения возвращают
 * адаптер к веб-поиску. Ключи выдаёт панель разработчика Spotify.
 */
export async function setSpotifyCredentials(
  clientID: string,
  clientSecret: string,
): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem("cryon.spotify.clientID", clientID);
    localStorage.setItem("cryon.spotify.clientSecret", clientSecret);
    return;
  }
  await App.SetSpotifyCredentials(clientID, clientSecret);
}

/** Заданы ли ключи Spotify (сами ключи не раскрываются). */
export async function spotifyConnected(): Promise<boolean> {
  if (!isWailsRuntime()) {
    return Boolean(localStorage.getItem("cryon.spotify.clientID"));
  }
  try {
    return await App.SpotifyConnected();
  } catch {
    return false;
  }
}

/**
 * Сохранить и применить client_id SoundCloud. Необязателен — client_id
 * определяется автоматически, — но заданный вручную надёжнее. Пустая строка
 * возвращает адаптер к автоопределению.
 */
export async function setSoundCloudClientID(clientID: string): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem("cryon.soundcloud.clientID", clientID);
    return;
  }
  await App.SetSoundCloudClientID(clientID);
}

/** Задан ли client_id SoundCloud вручную. */
export async function soundcloudConnected(): Promise<boolean> {
  if (!isWailsRuntime()) {
    return Boolean(localStorage.getItem("cryon.soundcloud.clientID"));
  }
  try {
    return await App.SoundCloudClientIDSet();
  } catch {
    return false;
  }
}

/**
 * Сохранить и применить ключ YouTube Data API. Необязателен — поиск работает
 * через Bing и парсинг, — но с ключом выдача стабильнее. Пустая строка
 * возвращает бесключевой режим.
 */
export async function setYouTubeAPIKey(apiKey: string): Promise<void> {
  if (!isWailsRuntime()) {
    localStorage.setItem("cryon.youtube.apiKey", apiKey);
    return;
  }
  await App.SetYouTubeAPIKey(apiKey);
}

/** Задан ли ключ YouTube Data API. */
export async function youtubeConnected(): Promise<boolean> {
  if (!isWailsRuntime()) {
    return Boolean(localStorage.getItem("cryon.youtube.apiKey"));
  }
  try {
    return await App.YouTubeAPIKeySet();
  } catch {
    return false;
  }
}

/** Подписка на успешный вход в Yandex. Возвращает функцию отписки. */
export function onYandexConnected(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("yandex:connected", handler);
  });
  return () => off();
}

/**
 * Подписка на смену ключа Last.fm (событие Wails "lastfm:changed"). Главная
 * использует её, чтобы перезагрузить подборки сразу после сохранения ключа,
 * а не после перезапуска. Возвращает функцию отписки.
 */
export function onLastFMChanged(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("lastfm:changed", handler);
  });
  return () => off();
}

/** Подписка на изменение реального статуса источников. */
export function onSourceStatusChanged(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("source:status-changed", handler);
  });
  return () => off();
}

/** Подписка на изменение списка избранного. */
export function onFavoritesChanged(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("favorites:changed", handler);
  });
  return () => off();
}

/** Подписка на изменение списка оповещений (backend пишет в SQLite и эмитит). */
export function onNotificationsChanged(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("notifications:changed", handler);
  });
  return () => off();
}

/**
 * Подписка на изменение профиля вкусов (оценка рекомендации). Экраны с
 * подборками по нему сбрасывают кэш: оценка должна отражаться сразу, а не
 * после истечения TTL готовой подборки на backend.
 */
export function onRecoChanged(handler: () => void): () => void {
  if (!isWailsRuntime()) return () => {};
  let off = () => {};
  void import("../../../wailsjs/runtime/runtime").then(({ EventsOn }) => {
    off = EventsOn("reco:changed", handler);
  });
  return () => off();
}

/** Реальный статус подключения источников для боковой панели. */
/** Уровень готовности источника: полностью/частично/выключен. */
export type SourceLevel = "ok" | "limited" | "off";

export interface SourceStatusDto {
  id: SourceId;
  connected: boolean;
  level: SourceLevel;
  error?: string;
}

function normalizeLevel(level: unknown, connected: boolean): SourceLevel {
  if (level === "ok" || level === "limited" || level === "off") return level;
  // Обратная совместимость со старым бэкендом, отдававшим только connected.
  return connected ? "ok" : "off";
}

export async function listSourceStatus(): Promise<SourceStatusDto[]> {
  if (!isWailsRuntime()) {
    return [
      { id: "youtube", connected: true, level: "ok" },
      { id: "soundcloud", connected: true, level: "ok" },
      { id: "spotify", connected: true, level: "limited" },
      { id: "yandex", connected: false, level: "limited" },
      { id: "vk", connected: false, level: "off" },
      { id: "local", connected: false, level: "off" },
    ];
  }
  try {
    const rows = await App.ListSourceStatus();
    if (!Array.isArray(rows)) return [];
    return rows.map((r) => {
      const connected = Boolean(r.connected);
      return {
        id: r.id as SourceId,
        connected,
        level: normalizeLevel((r as { level?: unknown }).level, connected),
        error: typeof (r as unknown as { error?: unknown }).error === "string" ? (r as unknown as { error: string }).error : undefined,
      };
    });
  } catch {
    return [];
  }
}

/** Проверить сохранённые учётные данные источника реальным API. */
export async function validateSource(source: SourceId): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.ValidateSource(source);
}

// --- История прослушивания ---

export interface HistoryEntryDto {
  track: Track;
  playedAt: string;
}

const historyListeners = new Set<() => void>();

/** Подписка на изменения истории, в том числе в web fallback. */
export function onHistoryChanged(listener: () => void): () => void {
  historyListeners.add(listener);
  return () => historyListeners.delete(listener);
}

function emitHistoryChanged() {
  historyListeners.forEach((listener) => listener());
}

/** Записать трек в историю прослушивания. */
export async function addHistory(track: Track): Promise<void> {
  if (!isWailsRuntime()) {
    const history = readMockHistory();
    saveMockHistory([
      { track: { ...track }, playedAt: new Date().toISOString() },
      ...history.filter((entry) => entry.track.id !== track.id),
    ]);
    emitHistoryChanged();
    return;
  }
  await App.AddHistory(toBackendTrack(track));
  emitHistoryChanged();
}

/** История прослушивания: список записей (свежие сверху). */
export async function listHistory(): Promise<HistoryEntryDto[]> {
  if (!isWailsRuntime()) return readMockHistory().map((entry) => ({ ...entry, track: { ...entry.track } }));
  const rows = await App.ListHistory();
  if (!Array.isArray(rows)) return [];
  return rows.map((r) => ({
    track: mapTrack(r.track),
    playedAt: new Date(r.playedAtMs).toISOString(),
  }));
}

/** Очистить историю прослушивания. */
export async function clearHistory(): Promise<void> {
  if (!isWailsRuntime()) {
    saveMockHistory([]);
    emitHistoryChanged();
    return;
  }
  await App.ClearHistory();
  emitHistoryChanged();
}

// --- Оповещения (колокольчик) ---

export interface NotificationDto {
  id: string;
  kind: "info" | "success" | "warning" | "error";
  title: string;
  message?: string;
  createdAt: string;
  read: boolean;
}

/** Список оповещений из SQLite (свежие сверху), не более 50. */
export async function listNotifications(): Promise<NotificationDto[]> {
  if (!isWailsRuntime()) return [];
  const rows = await App.ListNotifications();
  if (!Array.isArray(rows)) return [];
  return rows.map((r) => ({
    id: r.id,
    kind: (r.kind as NotificationDto["kind"]) || "info",
    title: r.title,
    message: r.message || undefined,
    createdAt: new Date(r.createdAtMs).toISOString(),
    read: Boolean(r.read),
  }));
}

/** Добавить оповещение (например, об ошибке воспроизведения с фронтенда). */
export async function addNotification(
  kind: NotificationDto["kind"],
  title: string,
  message?: string,
): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.AddNotification(kind, title, message ?? "");
}

/** Пометить все оповещения прочитанными. */
export async function markAllNotificationsRead(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.MarkAllNotificationsRead();
}

/** Удалить одно оповещение по id. */
export async function removeNotification(id: string): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.RemoveNotification(id);
}

/** Удалить все оповещения. */
export async function clearNotifications(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.ClearNotifications();
}

/**
 * Полный сброс всех пользовательских данных (история, избранное, плейлисты,
 * локальная библиотека, настройки, токены, кэш рекомендаций). Возвращает к
 * «чистому аккаунту». NOTE: добавлено koda.
 */
export async function resetAllData(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.ResetAllData();
}

// --- Пользовательские плейлисты ---

export interface UserPlaylistDto {
  id: string;
  title: string;
  description: string;
  trackCount: number;
  /** Обложки первых треков (до 4) для коллажа на карточке плейлиста. */
  coverUrls: string[];
}

function mapPlaylist(p: {
  id: string;
  title: string;
  description: string;
  trackCount: number;
  coverUrls?: string[];
}): UserPlaylistDto {
  return {
    id: p.id,
    title: p.title,
    description: p.description ?? "",
    trackCount: p.trackCount ?? 0,
    coverUrls: Array.isArray(p.coverUrls) ? p.coverUrls : [],
  };
}

/** Список пользовательских плейлистов. */
export async function listPlaylists(): Promise<UserPlaylistDto[]> {
  if (!isWailsRuntime()) return mockUserPlaylists.map((playlist) => ({ ...playlist, coverUrls: [...playlist.coverUrls] }));
  const rows = await App.ListPlaylists();
  if (!Array.isArray(rows)) return [];
  return rows.map(mapPlaylist);
}

/** Создать плейлист. */
export async function createPlaylist(
  title: string,
  description = "",
): Promise<UserPlaylistDto | null> {
  if (!isWailsRuntime()) {
    const playlist: UserPlaylistDto = {
      id: `mock-playlist-${++mockPlaylistSequence}`,
      title: title.trim(),
      description,
      trackCount: 0,
      coverUrls: [],
    };
    mockUserPlaylists = [playlist, ...mockUserPlaylists];
    mockPlaylistTracks.set(playlist.id, []);
    return { ...playlist, coverUrls: [] };
  }
  const p = await App.CreatePlaylist(title, description);
  return mapPlaylist(p);
}

/** Обновить название и описание плейлиста. */
export async function updatePlaylist(
  id: string,
  title: string,
  description: string,
): Promise<void> {
  if (!isWailsRuntime()) {
    mockUserPlaylists = mockUserPlaylists.map((playlist) =>
      playlist.id === id ? { ...playlist, title: title.trim(), description } : playlist,
    );
    return;
  }
  await App.UpdatePlaylist(id, title, description);
}

/** Удалить плейлист. */
export async function deletePlaylist(id: string): Promise<void> {
  if (!isWailsRuntime()) {
    mockUserPlaylists = mockUserPlaylists.filter((playlist) => playlist.id !== id);
    mockPlaylistTracks.delete(id);
    return;
  }
  await App.DeletePlaylist(id);
}

/** Треки плейлиста. */
export async function listPlaylistTracks(playlistId: string): Promise<Track[]> {
  if (!isWailsRuntime()) return mockPlaylistTracks.get(playlistId) ?? [];
  return mapTracks(await App.ListPlaylistTracks(playlistId));
}

/** Добавить трек в плейлист. */
export async function addTrackToPlaylist(
  playlistId: string,
  track: Track,
): Promise<void> {
  if (!isWailsRuntime()) {
    const tracks = mockPlaylistTracks.get(playlistId) ?? [];
    if (!tracks.some((item) => item.id === track.id)) {
      mockPlaylistTracks.set(playlistId, [...tracks, track]);
      mockUserPlaylists = mockUserPlaylists.map((playlist) =>
        playlist.id === playlistId
          ? {
              ...playlist,
              trackCount: tracks.length + 1,
              coverUrls: track.coverUrl ? [track.coverUrl, ...playlist.coverUrls].slice(0, 4) : playlist.coverUrls,
            }
          : playlist,
      );
    }
    return;
  }
  await App.AddTrackToPlaylist(playlistId, toBackendTrack(track));
}

/** Убрать трек из плейлиста. */
export async function removeTrackFromPlaylist(
  playlistId: string,
  compositeId: string,
): Promise<void> {
  if (!isWailsRuntime()) return;
  const { source, rawId } = splitTrackId(compositeId);
  await App.RemoveTrackFromPlaylist(playlistId, source, rawId);
}

/**
 * Задать новый порядок треков плейлиста (drag-and-drop).
 * orderedIds — составные id треков (`source:rawId`) в желаемом порядке.
 */
export async function reorderPlaylistTracks(
  playlistId: string,
  orderedIds: string[],
): Promise<void> {
  if (!isWailsRuntime()) return;
  const keys = orderedIds.map((compositeId) => {
    const { source, rawId } = splitTrackId(compositeId);
    return { service: source, trackId: rawId };
  });
  await App.ReorderPlaylistTracks(playlistId, keys);
}

// --- Воспроизведение через backend (mpv) ---

/** Доступно ли воспроизведение через mpv. Если нет — играем в <audio>. */
export async function playerBackendAvailable(): Promise<boolean> {
  if (!isWailsRuntime()) return false;
  try {
    return await App.PlayerBackendAvailable();
  } catch {
    return false;
  }
}

export async function playerPlay(compositeId: string, streamUrl: string): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerPlay(compositeId, streamUrl);
}

export async function playerPause(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerPause();
}

export async function playerResume(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerResume();
}

export async function playerStop(): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerStop();
}

export async function playerSeek(positionS: number): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerSeek(positionS);
}

export async function playerSetVolume(volume: number): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerSetVolume(volume);
}

/**
 * Задаёт усиления полос эквалайзера (дБ) для mpv-движка. Частоты фиксированы
 * на стороне backend (см. eqFreqs в internal/playback). Пустой массив/все нули
 * снимают фильтр. Вне Wails — no-op (эквалайзер применяется через Web Audio).
 */
export async function playerSetEqualizer(gains: number[]): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerSetEqualizer(gains);
}

/**
 * Включает/выключает нормализацию громкости (dynaudnorm) для mpv-движка.
 * Вне Wails — no-op (нормализация встроенного плеера идёт через Web Audio).
 */
export async function playerSetNormalize(enabled: boolean): Promise<void> {
  if (!isWailsRuntime()) return;
  await App.PlayerSetNormalize(enabled);
}

/**
 * Полный статус плеера из mpv. Возвращает null, если статус получить
 * не удалось (например, mpv завершился по достижении конца трека).
 */
export async function playerStatus(): Promise<{
  state: string;
  positionS: number;
  durationS: number;
  eof: boolean;
} | null> {
  if (!isWailsRuntime()) return null;
  try {
    const status = await App.PlayerStatus();
    return {
      state: status?.state ?? "idle",
      positionS: status?.positionS ?? 0,
      durationS: status?.durationS ?? 0,
      eof: status?.eof ?? false,
    };
  } catch {
    return null;
  }
}

/** Текст песни из lrclib.net. */
export interface LyricsResult {
  /** Синхронизированный текст (LRC с таймкодами) или "". */
  synced: string;
  /** Обычный текст без таймкодов или "". */
  plain: string;
  /** Песня инструментальная (текста нет намеренно). */
  instrumental: boolean;
  /** Нашёлся ли текст (не путать с инструментальным). */
  found: boolean;
}

const EMPTY_LYRICS: LyricsResult = { synced: "", plain: "", instrumental: false, found: false };

/**
 * Загружает текст песни (по возможности синхронизированный) из lrclib.net.
 * Вне Wails и при сбое — пустой результат (found:false), не бросает.
 */
export async function getLyrics(
  artist: string,
  title: string,
  album: string,
  durationS: number,
): Promise<LyricsResult> {
  if (!isWailsRuntime()) return EMPTY_LYRICS;
  try {
    const r = await App.GetLyrics(artist, title, album, Math.round(durationS) || 0);
    return {
      synced: r?.synced ?? "",
      plain: r?.plain ?? "",
      instrumental: Boolean(r?.instrumental),
      found: Boolean(r?.found),
    };
  } catch {
    return EMPTY_LYRICS;
  }
}
