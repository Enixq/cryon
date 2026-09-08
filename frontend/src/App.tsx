import { Navigate, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { currentAccount } from "./shared/api/client";
import { AuthPage } from "./pages/AuthPage";
import { AppLayout } from "./widgets/AppLayout";
import { HomePage } from "./pages/HomePage";
import { SearchPage } from "./pages/SearchPage";
import { LibraryPage } from "./pages/LibraryPage";
import { PlaylistsPage } from "./pages/PlaylistsPage";
import { PlaylistDetailPage } from "./pages/PlaylistDetailPage";
import { AlbumDetailPage } from "./pages/AlbumDetailPage";
import { LocalMusicPage } from "./pages/LocalMusicPage";
import { FavoritesPage } from "./pages/FavoritesPage";
import { HistoryPage } from "./pages/HistoryPage";
import { SettingsPage } from "./pages/SettingsPage";
import { ServicesPage } from "./pages/ServicesPage";
import { CollectionPage } from "./pages/CollectionPage";
import { SmartPlaylistPage } from "./pages/SmartPlaylistPage";
import { GenrePage } from "./pages/GenrePage";
import { ArtistPage } from "./pages/ArtistPage";

function App() {
  const [ready, setReady] = useState(false); const [authenticated, setAuthenticated] = useState(false);
  useEffect(() => { void currentAccount().then((account) => { setAuthenticated(Boolean(account)); setReady(true); }); }, []);
  if (!ready) return <div className="min-h-screen bg-[#090b14]" />;
  if (!authenticated) return <AuthPage onAuthenticated={() => setAuthenticated(true)} />;
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route path="/" element={<HomePage />} />
        <Route path="/search" element={<SearchPage />} />
        <Route path="/library" element={<LibraryPage />} />
        <Route path="/playlists" element={<PlaylistsPage />} />
        <Route path="/playlists/:id" element={<PlaylistDetailPage />} />
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
  );
}

export default App;
