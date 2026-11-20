import { Navigate, Route, Routes } from "react-router-dom";
import { lazy, Suspense, useEffect, useState } from "react";
import { currentAccount } from "./shared/api/client";
import { AuthPage } from "./pages/AuthPage";
import { AppLayout } from "./widgets/AppLayout";
import { apiV1BaseUrl, hasApiV1Runtime } from "./shared/api/v1";

const HomePage = lazy(async () => ({ default: (await import("./pages/HomePage")).HomePage }));
const SearchPage = lazy(async () => ({ default: (await import("./pages/SearchPage")).SearchPage }));
const LibraryPage = lazy(async () => ({ default: (await import("./pages/LibraryPage")).LibraryPage }));
const PlaylistsPage = lazy(async () => ({ default: (await import("./pages/PlaylistsPage")).PlaylistsPage }));
const PlaylistDetailPage = lazy(async () => ({ default: (await import("./pages/PlaylistDetailPage")).PlaylistDetailPage }));
const RemotePlaylistPage = lazy(() => import("./pages/RemotePlaylistPage"));
const AlbumDetailPage = lazy(() => import("./pages/AlbumDetailPage"));
const LocalMusicPage = lazy(async () => ({ default: (await import("./pages/LocalMusicPage")).LocalMusicPage }));
const FavoritesPage = lazy(async () => ({ default: (await import("./pages/FavoritesPage")).FavoritesPage }));
const HistoryPage = lazy(async () => ({ default: (await import("./pages/HistoryPage")).HistoryPage }));
const SettingsPage = lazy(async () => ({ default: (await import("./pages/SettingsPage")).SettingsPage }));
const ServicesPage = lazy(async () => ({ default: (await import("./pages/ServicesPage")).ServicesPage }));
const CollectionPage = lazy(() => import("./pages/CollectionPage"));
const SmartPlaylistPage = lazy(() => import("./pages/SmartPlaylistPage"));
const GenrePage = lazy(() => import("./pages/GenrePage"));
const ArtistPage = lazy(() => import("./pages/ArtistPage"));

function RouteLoading() {
  return <div className="min-h-[50vh]" aria-busy="true" />;
}

function App() {
  const [ready, setReady] = useState(false); const [authenticated, setAuthenticated] = useState(false);
  const [startupError, setStartupError] = useState("");
  useEffect(() => {
    void (async () => {
      try {
        if (hasApiV1Runtime()) {
          const response = await fetch(apiV1BaseUrl() + "/v1/system/health");
          if (!response.ok) {
            const body = await response.json() as { error?: string };
            throw new Error(body.error || "Не удалось открыть библиотеку Cryon");
          }
        }
        const account = await currentAccount();
        setAuthenticated(Boolean(account));
      } catch (error) {
        setStartupError(error instanceof Error ? error.message : "Не удалось запустить Cryon");
      } finally {
        setReady(true);
      }
    })();
  }, []);
  if (!ready) return <div className="min-h-screen bg-[#090b14]" />;
  if (startupError) return <main className="grid min-h-screen place-items-center bg-[#090b14] p-6 text-slate-200"><div className="max-w-lg rounded-2xl border border-rose-400/30 bg-white/5 p-6"><h1 className="text-xl font-semibold text-white">Не удалось запустить Cryon</h1><p className="mt-3 text-sm">{startupError}</p><p className="mt-3 text-sm text-slate-400">Ваши данные сохранены. Проверьте версию приложения и попробуйте снова.</p></div></main>;
  if (!authenticated) return <AuthPage onAuthenticated={() => setAuthenticated(true)} />;
  return (
    <Suspense fallback={<RouteLoading />}>
      <Routes>
        <Route element={<AppLayout />}>
          <Route path="/" element={<HomePage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/library" element={<LibraryPage />} />
          <Route path="/playlists" element={<PlaylistsPage />} />
          <Route path="/playlists/:id" element={<PlaylistDetailPage />} />
          <Route path="/remote-playlists/:source/:id" element={<RemotePlaylistPage />} />
          <Route path="/albums/:id" element={<AlbumDetailPage />} />
          <Route path="/local" element={<LocalMusicPage />} />
          <Route path="/favorites" element={<FavoritesPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/collection/:kind" element={<CollectionPage />} />
          <Route path="/smart/:id" element={<SmartPlaylistPage />} />
          <Route path="/genre/:slug" element={<GenrePage />} />
          <Route path="/artist/:name" element={<ArtistPage />} />
          <Route path="/services" element={<ServicesPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </Suspense>
  );
}

export default App;
