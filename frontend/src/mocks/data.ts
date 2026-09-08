import type {
  Album,
  Artist,
  HistoryEntry,
  Playlist,
  QuickMix,
  ServiceState,
  Track,
} from "../shared/types";

// Все данные ниже — временные моки для сборки полного фронтенда.
// Backend позже заменит их реальными данными из источников.

export const quickMixes: QuickMix[] = [
  { id: "m1", title: "Мой микс", subtitle: "На основе ваших прослушиваний", accent: "violet" },
  { id: "m2", title: "Радар новинок", subtitle: "Свежие релизы каждую пятницу", accent: "cyan" },
  { id: "m3", title: "Любимые треки", subtitle: "Треки, которые ты любишь", accent: "pink" },
  { id: "m4", title: "Снято с повторов", subtitle: "Открой что-то новое", accent: "violet" },
];

// Временные обложки-заглушки (детерминированные по seed).
// Backend позже заменит их реальными URL обложек из источников.
const cover = (seed: string) => `https://picsum.photos/seed/${seed}/300/300`;

export const tracks: Track[] = [
  { id: "t1", title: "After Dark", artist: "Mr.Kitty", album: "Time", source: "youtube", duration: 241, accent: "violet", liked: true, coverUrl: cover("afterdark") },
  { id: "t2", title: "Sweater Weather", artist: "The Neighbourhood", album: "I Love You.", source: "soundcloud", duration: 229, accent: "slate", coverUrl: cover("sweater") },
  { id: "t3", title: "Пачка сигарет", artist: "Кино", album: "Звезда по имени Солнце", source: "vk", duration: 269, accent: "amber", coverUrl: cover("kino1") },
  { id: "t4", title: "Звезда по имени Солнце", artist: "Кино", album: "Звезда по имени Солнце", source: "youtube", duration: 238, accent: "orange", coverUrl: cover("kino2") },
  { id: "t5", title: "Smells Like Teen Spirit", artist: "Nirvana", album: "Nevermind", source: "youtube", duration: 301, accent: "cyan", coverUrl: cover("nirvana") },
  { id: "t6", title: "Nightcall", artist: "Kavinsky", album: "OutRun", source: "local", duration: 257, accent: "rose", coverUrl: cover("nightcall") },
  { id: "t7", title: "Midnight City", artist: "M83", album: "Hurry Up, We're Dreaming", source: "youtube", duration: 243, accent: "cyan", coverUrl: cover("m83") },
  { id: "t8", title: "Resonance", artist: "HOME", album: "Odyssey", source: "soundcloud", duration: 212, accent: "emerald", liked: true, coverUrl: cover("resonance") },
  { id: "t9", title: "Blinding Lights", artist: "The Weeknd", album: "After Hours", source: "spotify", duration: 200, accent: "rose", coverUrl: cover("blinding") },
  { id: "t10", title: "The Perfect Girl", artist: "Mareux", album: "Lovers From The Past", source: "soundcloud", duration: 268, accent: "violet", coverUrl: cover("perfectgirl") },
  { id: "t11", title: "Кукушка", artist: "Кино", album: "Чёрный альбом", source: "yandex", duration: 275, accent: "pink", coverUrl: cover("kukushka") },
  { id: "t12", title: "Feel Good Inc.", artist: "Gorillaz", album: "Demon Days", source: "youtube", duration: 222, accent: "emerald", coverUrl: cover("gorillaz") },
];

export const recentTracks: Track[] = [tracks[0], tracks[1], tracks[2], tracks[3], tracks[4]];

export const recommendedPlaylists: Playlist[] = [
  { id: "p1", title: "Synthwave Chill", description: "Мягкий неон и поздний вечер", source: "spotify", accent: "violet", trackCount: 42, coverUrl: cover("synthwave") },
  { id: "p2", title: "Лёгкий рок 90-х", description: "Гитары и ностальгия", source: "yandex", accent: "cyan", trackCount: 58, coverUrl: cover("rock90") },
  { id: "p3", title: "Инди-открытия", description: "Новые находки на каждый день", source: "vk", accent: "pink", trackCount: 30, coverUrl: cover("indie") },
  { id: "p4", title: "Энергия утра", description: "Бодрый старт без суеты", source: "soundcloud", accent: "orange", trackCount: 24, coverUrl: cover("morning") },
  { id: "p5", title: "Вечерний вайб", description: "Спокойный неон перед сном", source: "youtube", accent: "violet", trackCount: 36, coverUrl: cover("evening") },
];

export const userPlaylists: Playlist[] = [
  { id: "up1", title: "Мой синтвейв", description: "Личная коллекция синтвейва", source: "local", accent: "violet", trackCount: 64, coverUrl: cover("mysynth") },
  { id: "up2", title: "На пробежку", description: "Быстрый темп и энергия", source: "spotify", accent: "rose", trackCount: 28, coverUrl: cover("running") },
  { id: "up3", title: "Работа и фокус", description: "Инструментал без слов", source: "youtube", accent: "cyan", trackCount: 45, coverUrl: cover("focus") },
  { id: "up4", title: "Русский рок", description: "Классика отечественного рока", source: "yandex", accent: "amber", trackCount: 51, coverUrl: cover("rusrock") },
  { id: "up5", title: "Дорожный плейлист", description: "Для долгих поездок", source: "soundcloud", accent: "emerald", trackCount: 39, coverUrl: cover("roadtrip") },
  { id: "up6", title: "Ночной чил", description: "Lo-fi и медленный бит", source: "local", accent: "pink", trackCount: 33, coverUrl: cover("nightchill") },
];

export const albums: Album[] = [
  { id: "a1", title: "Time", artist: "Mr.Kitty", year: 2014, source: "youtube", accent: "violet", trackCount: 12, coverUrl: cover("afterdark") },
  { id: "a2", title: "OutRun", artist: "Kavinsky", year: 2013, source: "local", accent: "rose", trackCount: 13, coverUrl: cover("nightcall") },
  { id: "a3", title: "Nevermind", artist: "Nirvana", year: 1991, source: "youtube", accent: "cyan", trackCount: 12, coverUrl: cover("nirvana") },
  { id: "a4", title: "Demon Days", artist: "Gorillaz", year: 2005, source: "spotify", accent: "emerald", trackCount: 15, coverUrl: cover("gorillaz") },
  { id: "a5", title: "After Hours", artist: "The Weeknd", year: 2020, source: "spotify", accent: "rose", trackCount: 14, coverUrl: cover("blinding") },
  { id: "a6", title: "Звезда по имени Солнце", artist: "Кино", year: 1989, source: "yandex", accent: "amber", trackCount: 10, coverUrl: cover("kino2") },
];

export const artists: Artist[] = [
  { id: "ar1", name: "Mr.Kitty", accent: "violet", followers: "1,2 млн", coverUrl: cover("artist-kitty") },
  { id: "ar2", name: "Кино", accent: "amber", followers: "4,8 млн", coverUrl: cover("artist-kino") },
  { id: "ar3", name: "The Weeknd", accent: "rose", followers: "82 млн", coverUrl: cover("artist-weeknd") },
  { id: "ar4", name: "Gorillaz", accent: "emerald", followers: "27 млн", coverUrl: cover("artist-gorillaz") },
  { id: "ar5", name: "M83", accent: "cyan", followers: "3,4 млн", coverUrl: cover("artist-m83") },
];

export const likedTracks: Track[] = tracks.filter((t) => t.liked).concat(tracks[6], tracks[9]);

// Треки из локальной библиотеки — для страницы локальной музыки.
// Мок: показываем локальный трек плюс срез общей коллекции как "просканированные" файлы.
export const localTracks: Track[] = [
  tracks[5], // Nightcall (local)
  tracks[0],
  tracks[4],
  tracks[9],
  tracks[7],
  tracks[11],
];

export interface LocalFolder {
  id: string;
  path: string;
  trackCount: number;
}

export const localFolders: LocalFolder[] = [
  { id: "f1", path: "D:\\Музыка\\Synthwave", trackCount: 128 },
  { id: "f2", path: "D:\\Музыка\\Рок", trackCount: 342 },
  { id: "f3", path: "C:\\Users\\Alex\\Music\\Загрузки", trackCount: 57 },
];

export const services: ServiceState[] = [
  { id: "spotify", name: "Spotify", connected: true },
  { id: "soundcloud", name: "SoundCloud", connected: true },
  { id: "vk", name: "VK Музыка", connected: true },
  { id: "yandex", name: "Яндекс Музыка", connected: false },
  { id: "youtube", name: "YouTube Music", connected: true },
  { id: "local", name: "Локальная музыка", connected: true },
];

function hoursAgo(hours: number): string {
  return new Date(Date.now() - hours * 3600_000).toISOString();
}

export const history: HistoryEntry[] = [
  { track: tracks[0], playedAt: hoursAgo(1) },
  { track: tracks[7], playedAt: hoursAgo(2) },
  { track: tracks[4], playedAt: hoursAgo(3) },
  { track: tracks[2], playedAt: hoursAgo(6) },
  { track: tracks[9], playedAt: hoursAgo(20) },
  { track: tracks[3], playedAt: hoursAgo(26) },
  { track: tracks[11], playedAt: hoursAgo(30) },
  { track: tracks[5], playedAt: hoursAgo(50) },
];

export const initialQueue: Track[] = [tracks[0], tracks[1], tracks[2], tracks[3], tracks[4], tracks[7]];
