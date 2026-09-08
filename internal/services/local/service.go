package local

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/dhowden/tag"

	"Cryon2/internal/domain"
)

// Service — адаптер локальной музыки. Держит в памяти индекс
// просканированных треков и умеет искать по нему.
type Service struct {
	mu     sync.RWMutex
	byID   map[string]LocalTrack
	sorted []LocalTrack // отсортированный кэш для стабильной выдачи
}

var _ domain.MusicService = (*Service)(nil)

// New создаёт пустой локальный адаптер. Треки добавляются через Replace/Merge
// после сканирования папок.
func New() *Service {
	return &Service{byID: map[string]LocalTrack{}}
}

func (s *Service) ID() domain.ServiceID { return domain.ServiceLocal }

// Replace полностью заменяет индекс библиотеки набором треков.
func (s *Service) Replace(tracks []LocalTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID = make(map[string]LocalTrack, len(tracks))
	for _, t := range tracks {
		s.byID[t.ID] = t
	}
	s.rebuildLocked()
}

// Merge добавляет/обновляет треки, не удаляя ранее известные.
func (s *Service) Merge(tracks []LocalTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tracks {
		s.byID[t.ID] = t
	}
	s.rebuildLocked()
}

// rebuildLocked пересобирает отсортированный кэш. Вызывается под write-lock.
func (s *Service) rebuildLocked() {
	s.sorted = make([]LocalTrack, 0, len(s.byID))
	for _, t := range s.byID {
		s.sorted = append(s.sorted, t)
	}
	sort.Slice(s.sorted, func(i, j int) bool {
		if s.sorted[i].Title == s.sorted[j].Title {
			return s.sorted[i].ID < s.sorted[j].ID
		}
		return strings.ToLower(s.sorted[i].Title) < strings.ToLower(s.sorted[j].Title)
	})
}

// All возвращает всю локальную библиотеку в стабильном порядке.
func (s *Service) All() []LocalTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LocalTrack, len(s.sorted))
	copy(out, s.sorted)
	return out
}

// FilePath возвращает путь к файлу трека по его id.
func (s *Service) FilePath(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.byID[id]
	return t.FilePath, ok
}

// Search ищет по названию, исполнителю и альбому (подстрока, без регистра).
func (s *Service) Search(_ context.Context, query string) ([]domain.Track, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Track, 0, 16)
	for _, t := range s.sorted {
		if q == "" || matches(t, q) {
			out = append(out, t.Track)
		}
	}
	return out, nil
}

func matches(t LocalTrack, q string) bool {
	if strings.Contains(strings.ToLower(t.Title), q) {
		return true
	}
	if strings.Contains(strings.ToLower(t.Album), q) {
		return true
	}
	for _, a := range t.Artists {
		if strings.Contains(strings.ToLower(a), q) {
			return true
		}
	}
	return false
}

// Cover читает обложку трека из аудиофайла. Возвращает байты и MIME-тип.
// Если обложки нет, второй результат — пустая строка.
func (s *Service) Cover(id string) ([]byte, string, error) {
	path, ok := s.FilePath(id)
	if !ok {
		return nil, "", os.ErrNotExist
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil {
		return nil, "", err
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, "", nil
	}
	mime := pic.MIMEType
	if mime == "" {
		mime = "image/jpeg"
	}
	return pic.Data, mime, nil
}
