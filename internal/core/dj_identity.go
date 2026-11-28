package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
	"context"
)

func (a *App) mergeDJPool(ctx context.Context, pool, exclude []domain.Track, provenance map[string][]intelligence.Channel, origins map[string][]intelligence.CandidateOrigin) ([]domain.Track, map[string][]intelligence.Channel, map[string][]intelligence.CandidateOrigin) {
	for _, g := range identity.GroupTracks(pool) {
		channels := []intelligence.Channel{}
		sources := []intelligence.CandidateOrigin{}
		seen := map[intelligence.Channel]bool{}
		for _, c := range g.Candidates {
			for _, ch := range provenance[djTrackID(c.Track)] {
				if !seen[ch] {
					channels = append(channels, ch)
					seen[ch] = true
				}
			}
			sources = append(sources, origins[djTrackID(c.Track)]...)
		}
		for _, c := range g.Candidates {
			provenance[djTrackID(c.Track)] = channels
			origins[djTrackID(c.Track)] = sources
		}
	}
	return a.reco.PrepareDJPool(ctx, pool, exclude, 80), provenance, origins
}
