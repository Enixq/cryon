package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"Cryon2/internal/update"
)

// APIHandler exposes the versioned HTTP API used by new native clients. It is
// intentionally separate from the legacy reflection RPC bridge so callers can
// migrate endpoint by endpoint without changing the current Wails frontend.
func (a *App) APIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/system/health", a.handleAPIHealth)
	mux.HandleFunc("/v1/system/update", a.handleAPIUpdate)
	mux.HandleFunc("/v1/events", a.handleAPIEvents)
	mux.HandleFunc("/v1/auth/yandex/device/start", a.handleYandexDeviceStart)
	mux.HandleFunc("/v1/auth/yandex/device/poll", a.handleYandexDevicePoll)
	mux.HandleFunc("/v1/devices/discover", a.handleAPIDeviceDiscover)
	mux.HandleFunc("/v1/devices/state", a.handleAPIDeviceState)
	mux.HandleFunc("/v1/sources", a.handleAPISources)
	mux.HandleFunc("/v1/sources/", a.handleAPISourceByID)
	mux.HandleFunc("/v1/search", a.handleAPISearch)
	mux.HandleFunc("/v1/catalog/playlists/search", a.handleAPICatalogPlaylistSearch)
	mux.HandleFunc("/v1/catalog/playlists/", a.handleAPICatalogPlaylistTracks)
	mux.HandleFunc("/v1/recommendations", a.handleAPIRecommendations)
	mux.HandleFunc("/v1/recommendations/", a.handleAPIRecommendationByKind)
	mux.HandleFunc("/v1/library/favorites", a.handleAPIFavorites)
	mux.HandleFunc("/v1/library/feedback", a.handleAPILibraryFeedback)
	mux.HandleFunc("/v1/library/history", a.handleAPIHistory)
	mux.HandleFunc("/v1/library/local/tracks", a.handleAPILocalTracks)
	mux.HandleFunc("/v1/library/local/folders", a.handleAPILocalFolders)
	mux.HandleFunc("/v1/playlists", a.handleAPIPlaylists)
	mux.HandleFunc("/v1/playlists/", a.handleAPIPlaylistByID)
	mux.HandleFunc("/v1/jobs", a.handleAPIJobs)
	mux.HandleFunc("/v1/imports/spotify", a.handleAPISpotifyImport)
	mux.HandleFunc("/v1/playback/status", a.handleAPIPlaybackStatus)
	mux.HandleFunc("/v1/playback/preferences", a.handleAPIPlaybackPreferences)
	mux.HandleFunc("/v1/playback/resolve", a.handleAPIPlaybackResolve)
	mux.HandleFunc("/v1/playback/fallback", a.handleAPIPlaybackFallback)
	mux.HandleFunc("/v1/playback/", a.handleAPIPlaybackCommand)
	mux.HandleFunc("/v1/jobs/", a.handleAPIJobByID)
	return mux
}

type apiError struct {
	Error string `json:"error"`
}

func (a *App) handleAPIDeviceDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	writeAPIJSON(w, http.StatusOK, discoverDevices(ctx, r.URL.Query().Get("ipv4")))
}

func (a *App) handleAPIUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	release, err := update.Check(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, release)
}

type apiHealthResponse struct {
	Status string `json:"status"`
	API    string `json:"api"`
	AtMs   int64  `json:"atMs"`
}

type apiSearchResponse struct {
	Query   string         `json:"query"`
	Service string         `json:"service,omitempty"`
	Tracks  []domain.Track `json:"tracks"`
}

type apiCatalogPlaylistSearchResponse struct {
	Playlists []domain.Playlist `json:"playlists"`
}

type apiPlaybackResolveRequest struct {
	Track  domain.Track `json:"track"`
	Native bool         `json:"native,omitempty"`
}

type apiPlaybackResolveResponse struct {
	Track     domain.Track `json:"track"`
	StreamURL string       `json:"streamUrl"`
	MimeType  string       `json:"mimeType,omitempty"`
}

type apiPlaybackFallbackRequest struct {
	Track              domain.Track `json:"track"`
	FailedService      string       `json:"failedService"`
	ExcludedCandidates []string     `json:"excludedCandidates,omitempty"`
}

type apiPlaybackFallbackResponse struct {
	Track     domain.Track `json:"track"`
	StreamURL string       `json:"streamUrl"`
	Reason    string       `json:"reason"`
}

type apiPlaybackPlayRequest struct {
	TrackID   string  `json:"trackId"`
	StreamURL string  `json:"streamUrl"`
	PositionS float64 `json:"positionS,omitempty"`
}

type apiPlaybackSeekRequest struct {
	PositionS float64 `json:"positionS"`
}

type apiPlaybackVolumeRequest struct {
	Volume int `json:"volume"`
}

type apiCreateJobRequest struct {
	Kind        string `json:"kind"`
	Priority    int    `json:"priority"`
	PayloadJSON string `json:"payloadJson"`
	MaxAttempts int    `json:"maxAttempts"`
}

type apiCreatePlaylistRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type apiUpdatePlaylistRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type apiPlaylistTrackRequest struct {
	Track domain.Track `json:"track"`
}

type apiPlaylistOrderRequest struct {
	Tracks []store.PlaylistTrackKey `json:"tracks"`
}

type apiRelatedTracksRequest struct {
	Track domain.Track `json:"track"`
	Limit int          `json:"limit"`
}

func (a *App) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	if a.startupError != "" {
		writeAPIError(w, http.StatusServiceUnavailable, "Не удалось открыть библиотеку: "+a.startupError)
		return
	}
	writeAPIJSON(w, http.StatusOK, apiHealthResponse{Status: "ok", API: "v1", AtMs: time.Now().UnixMilli()})
}

func (a *App) handleAPISpotifyImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "не удалось прочитать файл выгрузки")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "выберите файл выгрузки")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "не удалось прочитать файл выгрузки")
		return
	}
	tracks, err := parseSpotifyLibrary(data, strings.HasSuffix(strings.ToLower(header.Filename), ".zip"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.FormValue("action") == "preview" {
		writeAPITracks(w, tracks)
		return
	}
	if r.FormValue("action") != "import" {
		writeAPIError(w, http.StatusBadRequest, "неизвестное действие")
		return
	}
	var ids []string
	if err := json.Unmarshal([]byte(r.FormValue("trackIds")), &ids); err != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректный список треков")
		return
	}
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	filtered := make([]domain.Track, 0, len(ids))
	for _, track := range tracks {
		if selected[track.ID] {
			filtered = append(filtered, track)
		}
	}
	result := FavoritesImportResult{Source: string(domain.ServiceSpotify), Found: len(filtered)}
	if err := a.importFavoriteTracks(filtered, &result); err != nil {
		writeAPIAppError(w, err)
		return
	}
	if a.platform != nil {
		a.platform.Emit(favoritesChangedEvent)
	}
	writeAPIJSON(w, http.StatusOK, result)
}

func (a *App) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	a.eventHub().serve(w, r)
}

func (a *App) handleAPISources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	writeAPIJSON(w, http.StatusOK, a.ListSourceStatus())
}

func (a *App) handleAPISourceByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/sources/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "validate" {
		writeAPIError(w, http.StatusNotFound, "маршрут не найден")
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
		return
	}
	sourceID, err := url.PathUnescape(parts[0])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректный id источника")
		return
	}
	if _, ok := a.registry[domain.ServiceID(sourceID)]; !ok {
		writeAPIError(w, http.StatusNotFound, "источник не найден")
		return
	}
	if err := a.ValidateSource(sourceID); err != nil {
		writeAPIAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeAPIError(w, http.StatusBadRequest, "параметр q обязателен")
		return
	}
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	var (
		tracks []domain.Track
		err    error
	)
	if service == "" || service == "all" {
		tracks, err = a.SearchAll(query)
		service = ""
	} else {
		tracks, err = a.Search(service, query)
	}
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if tracks == nil {
		tracks = []domain.Track{}
	}
	writeAPIJSON(w, http.StatusOK, apiSearchResponse{Query: query, Service: service, Tracks: tracks})
}

func (a *App) handleAPICatalogPlaylistSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	if query == "" || service == "" {
		writeAPIError(w, http.StatusBadRequest, "q и service обязательны")
		return
	}
	playlists, err := a.SearchPlaylists(service, query)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, apiCatalogPlaylistSearchResponse{Playlists: playlists})
}

func (a *App) handleAPICatalogPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/catalog/playlists/"), "/"), "/")
	if len(parts) == 3 && parts[2] == "tracks" {
		parts = parts[:2]
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeAPIError(w, http.StatusBadRequest, "нужны service и playlistId")
		return
	}
	service, err1 := url.PathUnescape(parts[0])
	playlistID, err2 := url.PathUnescape(parts[1])
	if err1 != nil || err2 != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректный идентификатор плейлиста")
		return
	}
	tracks, err := a.GetPlaylistTracks(service, playlistID)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, tracks)
}

func (a *App) handleAPIRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	tracks, err := a.ListRecommendations(apiLimit(r, 20))
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPITracks(w, tracks)
}

func (a *App) handleAPIRecommendationByKind(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimPrefix(r.URL.Path, "/v1/recommendations/")
	if strings.Contains(kind, "/") || kind == "" {
		writeAPIError(w, http.StatusNotFound, "маршрут не найден")
		return
	}
	limit := apiLimit(r, 20)
	var (
		tracks []domain.Track
		err    error
	)
	switch kind {
	case "daily-mix":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
			return
		}
		tracks, err = a.ListDailyMix(limit)
	case "weekly-discoveries":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
			return
		}
		tracks, err = a.ListWeeklyDiscoveries(limit)
	case "new-releases":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
			return
		}
		tracks, err = a.ListNewReleases(limit)
	case "related":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
			return
		}
		var req apiRelatedTracksRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if err := validateAPITrack(req.Track); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Limit > 0 {
			limit = min(req.Limit, 100)
		}
		tracks, err = a.ListRelatedTracks(req.Track, limit)
	default:
		writeAPIError(w, http.StatusNotFound, "маршрут не найден")
		return
	}
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPITracks(w, tracks)
}

// Native notification actions use the same durable feedback and library as UI.
func (a *App) handleAPILibraryFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		state, err := a.RecoFeedbackState()
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, state)
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET или POST")
		return
	}
	var req struct {
		Track domain.Track `json:"track"`
		Score int          `json:"score"`
	}
	if err := decodeAPIJSON(w, r, &req); err != nil {
		return
	}
	if err := validateAPITrack(req.Track); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Score < -1 || req.Score > 1 {
		writeAPIError(w, http.StatusBadRequest, "некорректная оценка")
		return
	}
	if req.Score == 1 {
		if err := a.AddFavorite(req.Track); err != nil {
			writeAPIAppError(w, err)
			return
		}
	} else {
		if err := a.RemoveFavorite(string(req.Track.Service), req.Track.ID); err != nil {
			writeAPIAppError(w, err)
			return
		}
	}
	artist := strings.Join(req.Track.Artists, ", ")
	if len(req.Track.Artists) > 0 {
		artist = req.Track.Artists[0]
	}
	state, err := a.SetRecoFeedback(artist, req.Track.Title, req.Score)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, state)
}

func (a *App) handleAPIFavorites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	tracks, err := a.ListFavorites()
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if tracks == nil {
		tracks = []domain.Track{}
	}
	writeAPIJSON(w, http.StatusOK, tracks)
}

func (a *App) handleAPILocalTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	writeAPITracks(w, a.ListLocalTracks())
}

func (a *App) handleAPILocalFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	folders, err := a.ListLocalFolders()
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if folders == nil {
		folders = []string{}
	}
	writeAPIJSON(w, http.StatusOK, folders)
}

func (a *App) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		history, err := a.ListHistory()
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if history == nil {
			history = []HistoryItem{}
		}
		writeAPIJSON(w, http.StatusOK, history)
	case http.MethodPost:
		var req apiPlaylistTrackRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if err := validateAPITrack(req.Track); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.AddHistory(req.Track); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := a.ClearHistory(); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET, POST или DELETE")
	}
}

func (a *App) handleAPIPlaylists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		playlists, err := a.ListPlaylists()
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if playlists == nil {
			playlists = []domain.UserPlaylist{}
		}
		writeAPIJSON(w, http.StatusOK, playlists)
	case http.MethodPost:
		var req apiCreatePlaylistRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeAPIError(w, http.StatusBadRequest, "название плейлиста обязательно")
			return
		}
		playlist, err := a.CreatePlaylist(req.Title, strings.TrimSpace(req.Description))
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, http.StatusCreated, playlist)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET или POST")
	}
}

func (a *App) handleAPIPlaylistByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/playlists/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeAPIError(w, http.StatusNotFound, "плейлист не найден")
		return
	}
	playlistID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(playlistID) == "" {
		writeAPIError(w, http.StatusBadRequest, "некорректный id плейлиста")
		return
	}

	switch {
	case len(parts) == 1:
		a.handleAPIPlaylist(w, r, playlistID)
	case len(parts) == 2 && parts[1] == "tracks":
		a.handleAPIPlaylistTracks(w, r, playlistID)
	case len(parts) == 3 && parts[1] == "tracks" && parts[2] == "order":
		a.handleAPIPlaylistOrder(w, r, playlistID)
	default:
		writeAPIError(w, http.StatusNotFound, "маршрут не найден")
	}
}

func (a *App) handleAPIPlaylist(w http.ResponseWriter, r *http.Request, playlistID string) {
	switch r.Method {
	case http.MethodPatch:
		var req apiUpdatePlaylistRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeAPIError(w, http.StatusBadRequest, "название плейлиста обязательно")
			return
		}
		if err := a.UpdatePlaylist(playlistID, req.Title, strings.TrimSpace(req.Description)); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := a.DeletePlaylist(playlistID); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется PATCH или DELETE")
	}
}

func (a *App) handleAPIPlaylistTracks(w http.ResponseWriter, r *http.Request, playlistID string) {
	switch r.Method {
	case http.MethodGet:
		tracks, err := a.ListPlaylistTracks(playlistID)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if tracks == nil {
			tracks = []domain.Track{}
		}
		writeAPIJSON(w, http.StatusOK, tracks)
	case http.MethodPost:
		var req apiPlaylistTrackRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if err := validateAPITrack(req.Track); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.AddTrackToPlaylist(playlistID, req.Track); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		service := strings.TrimSpace(r.URL.Query().Get("service"))
		trackID := strings.TrimSpace(r.URL.Query().Get("trackId"))
		if service == "" || trackID == "" {
			writeAPIError(w, http.StatusBadRequest, "укажите service и trackId")
			return
		}
		if err := a.RemoveTrackFromPlaylist(playlistID, service, trackID); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET, POST или DELETE")
	}
}

func (a *App) handleAPIPlaylistOrder(w http.ResponseWriter, r *http.Request, playlistID string) {
	if r.Method != http.MethodPut {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется PUT")
		return
	}
	var req apiPlaylistOrderRequest
	if err := decodeAPIJSON(w, r, &req); err != nil {
		return
	}
	if err := a.ReorderPlaylistTracks(playlistID, req.Tracks); err != nil {
		writeAPIAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "хранилище недоступно")
		return
	}
	switch r.Method {
	case http.MethodGet:
		jobs, err := a.store.JobList(r.Context(), 100)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if jobs == nil {
			jobs = []store.Job{}
		}
		writeAPIJSON(w, http.StatusOK, jobs)
	case http.MethodPost:
		var req apiCreateJobRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		req.Kind = strings.TrimSpace(req.Kind)
		if req.Kind == "" {
			writeAPIError(w, http.StatusBadRequest, "тип задания обязателен")
			return
		}
		if !validJobKind(req.Kind) {
			writeAPIError(w, http.StatusBadRequest, "неизвестный тип задания")
			return
		}
		job, err := a.store.JobEnqueue(r.Context(), store.Job{
			Kind:        req.Kind,
			Priority:    req.Priority,
			PayloadJSON: req.PayloadJSON,
			MaxAttempts: req.MaxAttempts,
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if a.platform != nil {
			a.platform.Emit(jobsChangedEvent)
		}
		writeAPIJSON(w, http.StatusCreated, job)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET или POST")
	}
}

func (a *App) handleAPIJobByID(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "хранилище недоступно")
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"))
	if id == "" || strings.Contains(id, "/") {
		writeAPIError(w, http.StatusNotFound, "задание не найдено")
		return
	}
	if r.Method != http.MethodDelete {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется DELETE")
		return
	}
	if err := a.store.JobCancel(r.Context(), id); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	if a.platform != nil {
		a.platform.Emit(jobsChangedEvent)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAPIPlaybackResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
		return
	}
	var req apiPlaybackResolveRequest
	if err := decodeAPIJSON(w, r, &req); err != nil {
		return
	}
	if err := validateAPITrack(req.Track); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	track, err := a.ResolvePlayableTrack(req.Track)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if track.PlayableKind == domain.PlayableExternal {
		writeAPIError(w, http.StatusConflict, "прямой поток для трека недоступен")
		return
	}
	streamURL := apiTrackStreamURL(track)
	response := apiPlaybackResolveResponse{Track: track, StreamURL: streamURL}
	// Native Android playback must validate the provider URL before handing it
	// to Media3. Previously errors were swallowed and the endpoint returned a
	// local /stream URL anyway; the UI then reported a generic 502 and could not
	// start its alternate-source search. Desktop keeps the lightweight proxy URL
	// path and resolves quality when the desktop engine actually starts.
	if req.Native {
		stream, err := a.GetAudioStream(string(track.Service), track.ID)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if stream != nil {
			track.Quality = stream.Quality
			response.Track = track
		}
		if stream != nil && stream.Format == "application/x-mpegURL" && stream.URL != "" {
			response.StreamURL = stream.URL
			response.MimeType = stream.Format
		}
	}
	writeAPIJSON(w, http.StatusOK, response)
}

func (a *App) handleAPIPlaybackFallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
		return
	}
	var req apiPlaybackFallbackRequest
	if err := decodeAPIJSON(w, r, &req); err != nil {
		return
	}
	if err := validateAPITrack(req.Track); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.FailedService) == "" {
		req.FailedService = string(req.Track.Service)
	}
	if len(req.ExcludedCandidates) > 32 {
		writeAPIError(w, http.StatusBadRequest, "слишком много исключённых версий")
		return
	}
	candidate, ok := a.resolvePlaybackFallback(req.Track, req.FailedService, req.ExcludedCandidates)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "запасная версия трека не найдена")
		return
	}
	streamURL := apiTrackStreamURL(candidate.Track)
	writeAPIJSON(w, http.StatusOK, apiPlaybackFallbackResponse{
		Track:     candidate.Track,
		StreamURL: streamURL,
		Reason:    candidate.SelectionReason,
	})
}

func (a *App) handleAPIPlaybackStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET")
		return
	}
	if a.player == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "desktop-плеер недоступен")
		return
	}
	writeAPIJSON(w, http.StatusOK, a.PlayerStatus())
}

func (a *App) handleAPIPlaybackPreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeAPIJSON(w, http.StatusOK, a.GetPlaybackPreferences())
	case http.MethodPut:
		var preferences PlaybackPreferences
		if err := decodeAPIJSON(w, r, &preferences); err != nil {
			return
		}
		if err := a.SetPlaybackPreferences(preferences); err != nil {
			writeAPIAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется GET или PUT")
	}
}

func (a *App) handleAPIPlaybackCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "требуется POST")
		return
	}
	if a.player == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "desktop-плеер недоступен")
		return
	}

	command := strings.TrimPrefix(r.URL.Path, "/v1/playback/")
	var err error
	switch command {
	case "play":
		var req apiPlaybackPlayRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if strings.TrimSpace(req.TrackID) == "" || strings.TrimSpace(req.StreamURL) == "" {
			writeAPIError(w, http.StatusBadRequest, "trackId и streamUrl обязательны")
			return
		}
		if math.IsNaN(req.PositionS) || math.IsInf(req.PositionS, 0) || req.PositionS < 0 {
			writeAPIError(w, http.StatusBadRequest, "positionS должен быть неотрицательным числом")
			return
		}
		err = a.PlayerPlayAt(req.TrackID, req.StreamURL, req.PositionS)
	case "pause":
		err = a.PlayerPause()
	case "resume":
		err = a.PlayerResume()
	case "stop":
		err = a.PlayerStop()
	case "seek":
		var req apiPlaybackSeekRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if math.IsNaN(req.PositionS) || math.IsInf(req.PositionS, 0) || req.PositionS < 0 {
			writeAPIError(w, http.StatusBadRequest, "positionS должен быть неотрицательным числом")
			return
		}
		err = a.PlayerSeek(req.PositionS)
	case "volume":
		var req apiPlaybackVolumeRequest
		if err := decodeAPIJSON(w, r, &req); err != nil {
			return
		}
		if req.Volume < 0 || req.Volume > 100 {
			writeAPIError(w, http.StatusBadRequest, "volume должен быть от 0 до 100")
			return
		}
		err = a.PlayerSetVolume(req.Volume)
	default:
		writeAPIError(w, http.StatusNotFound, "команда плеера не найдена")
		return
	}
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func urlPathSegment(value string) string {
	return url.PathEscape(value)
}

func apiTrackStreamURL(track domain.Track) string {
	if track.Service == domain.ServiceLocal {
		return "/local/" + urlPathSegment(track.ID)
	}
	return "/stream/" + urlPathSegment(string(track.Service)) + "/" + urlPathSegment(track.ID)
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(target); err != nil {
		writeAPIError(w, http.StatusBadRequest, "некорректный JSON: "+err.Error())
		return err
	}
	return nil
}

func validateAPITrack(track domain.Track) error {
	if strings.TrimSpace(track.ID) == "" || strings.TrimSpace(string(track.Service)) == "" {
		return errors.New("укажите id и service трека")
	}
	return nil
}

func apiLimit(r *http.Request, fallback int) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return fallback
	}
	return min(limit, 100)
}

func writeAPITracks(w http.ResponseWriter, tracks []domain.Track) {
	if tracks == nil {
		tracks = []domain.Track{}
	}
	writeAPIJSON(w, http.StatusOK, tracks)
}

func writeAPIAppError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		writeAPIError(w, http.StatusRequestTimeout, "запрос отменён")
		return
	}
	writeAPIError(w, http.StatusInternalServerError, err.Error())
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeAPIJSON(w, status, apiError{Error: message})
}

func writeAPIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
