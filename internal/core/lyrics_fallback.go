package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/lyrics"
	"context"
	"strings"
	"time"
)

type trackLyricSource interface {
	TrackLyrics(context.Context, string) (lyrics.Result, error)
}

func (a *App) autoLyrics(ctx context.Context, track domain.Track) (lyrics.Result, error) {
	type reply struct {
		result lyrics.Result
		err    error
	}
	public := make(chan reply, 1)
	sources := make(chan lyrics.Result, 1)
	go func() {
		if a.lyrics == nil {
			public <- reply{}
			return
		}
		result, err := a.lyrics.Get(ctx, strings.Join(track.Artists, ", "), track.Title, track.Album, track.DurationMs/1000)
		public <- reply{result, err}
	}()
	go func() {
		for _, id := range []domain.ServiceID{track.Service, domain.ServiceYandex} {
			svc := a.registry[id]
			source, ok := svc.(trackLyricSource)
			if !ok {
				continue
			}
			attempt, cancel := context.WithTimeout(ctx, 7*time.Second)
			candidates := []domain.Track{track}
			if id != track.Service {
				found, err := a.cachedSourceSearch(attempt, svc, strings.Join(track.Artists, " ")+" "+track.Title)
				candidates = nil
				if err == nil {
					for _, candidate := range found {
						if domain.SameRecording(candidate, strings.Join(track.Artists, ", "), track.Title) {
							candidates = append(candidates, candidate)
						}
					}
				}
			}
			for _, candidate := range candidates {
				result, err := source.TrackLyrics(attempt, candidate.ID)
				if err == nil && result.Found {
					cancel()
					sources <- result
					return
				}
			}
			cancel()
		}
		sources <- lyrics.Result{}
	}()
	var lastErr error
	for i := 0; i < 2; i++ {
		select {
		case result := <-sources:
			if result.Found {
				return result, nil
			}
		case result := <-public:
			if result.result.Found {
				return result.result, nil
			}
			lastErr = result.err
		case <-ctx.Done():
			return lyrics.Result{}, ctx.Err()
		}
	}
	return lyrics.Result{}, lastErr
}
