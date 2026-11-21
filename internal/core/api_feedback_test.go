package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/store"
)

type notificationFeedbackStore struct {
	store.Store
	settings  map[string]string
	favorites map[string]domain.Track
}

func (s *notificationFeedbackStore) SettingGet(_ context.Context, key string) (string, bool, error) {
	value, ok := s.settings[key]
	return value, ok, nil
}
func (s *notificationFeedbackStore) SettingSet(_ context.Context, key, value string) error {
	s.settings[key] = value
	return nil
}
func (s *notificationFeedbackStore) FavoriteAdd(_ context.Context, track domain.Track) error {
	s.favorites[track.ID] = track
	return nil
}
func (s *notificationFeedbackStore) FavoriteRemove(_ context.Context, service domain.ServiceID, id string) error {
	delete(s.favorites, id)
	return nil
}

func TestNotificationFeedbackUpdatesLibraryAndPersistedRecommendations(t *testing.T) {
	s := &notificationFeedbackStore{settings: map[string]string{}, favorites: map[string]domain.Track{}}
	app := &App{ctx: context.Background(), store: s, reco: recommendations.New(s, nil, nil, nil), platform: nullHost{}}
	track := domain.Track{ID: "song", Service: domain.ServiceYouTube, Title: "Song", Artists: []string{"Artist", "Guest"}}
	post := func(score int, want int) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"track": track, "score": score})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/library/feedback", bytes.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("score %d: status %d body %s", score, rec.Code, rec.Body.String())
		}
	}
	post(1, http.StatusOK)
	post(1, http.StatusOK)
	if len(s.favorites) != 1 {
		t.Fatalf("like must save one library track: %v", s.favorites)
	}
	post(2, http.StatusBadRequest)
	if len(s.favorites) != 1 {
		t.Fatal("invalid rating changed the library")
	}
	post(-1, http.StatusOK)
	if len(s.favorites) != 0 {
		t.Fatal("disliked track remained in favorites")
	}
	// A fresh engine reads the durable feedback written by the notification.
	app.reco = recommendations.New(s, nil, nil, nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/library/feedback", nil))
	var scores map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &scores); err != nil {
		t.Fatal(err)
	}
	if scores["artist|song"] != -1 {
		t.Fatalf("dislike was not persisted for primary artist: %v", scores)
	}
}
