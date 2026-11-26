package recommendations

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"sort"
	"strings"

	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
)

// DJChannels reports only capabilities backed by this instance.
func (e *Engine) SetDJSourceSearch(sources func() []string, search func(context.Context, string, string) ([]domain.Track, error)) {
	e.djSources, e.djSearch = sources, search
}

func (e *Engine) DJChannels() []intelligence.ChannelBudget {
	out := []intelligence.ChannelBudget{{Channel: intelligence.Channel{Provider: "cryon", Capability: "discovery"}, MaxCandidates: 40}}
	if e.favFn != nil || e.histFn != nil {
		out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: "cryon", Capability: "library"}, MaxCandidates: 20})
	}
	if e.search != nil {
		out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: "cryon", Capability: "catalog_search"}, MaxCandidates: 20})
	}
	if e.djSources != nil && e.djSearch != nil {
		for _, source := range e.djSources() {
			out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: source, Capability: "catalog_search"}, MaxCandidates: 20})
		}
	}
	if e.lastfm.Available() {
		out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: "lastfm", Capability: "similar_tracks"}, MaxCandidates: 20})
	}
	if e.deezer != nil && e.deezer.Available() {
		out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: "deezer", Capability: "artist_tracks"}, MaxCandidates: 20})
	}
	if e.trackSimilar != nil && (e.trackSimilarAvailable == nil || e.trackSimilarAvailable()) {
		out = append(out, intelligence.ChannelBudget{Channel: intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"}, MaxCandidates: 20})
	}
	return out
}

// ExecuteDJStep uses the existing resolvers; the AI never receives a service
// client or playback handle. Query is plain metadata, never a URL or command.
func (e *Engine) ExecuteDJStep(ctx context.Context, step intelligence.SearchStep, seed domain.Track) ([]domain.Track, error) {
	var tracks []domain.Track
	var err error
	if step.Channel.Provider != "cryon" && step.Channel.Capability == "catalog_search" && e.djSearch != nil {
		if strings.Contains(step.Query, "://") {
			return nil, nil
		}
		tracks, err = e.djSearch(ctx, step.Channel.Provider, step.Query)
	} else {
		switch step.Channel {
		case (intelligence.Channel{Provider: "cryon", Capability: "library"}):
			if e.favFn != nil {
				tracks, err = e.favFn(ctx)
			}
			if e.histFn != nil {
				if rows, fetchErr := e.histFn(ctx, 100); fetchErr == nil {
					for _, row := range rows {
						tracks = append(tracks, row.Track)
					}
				}
			}
		case (intelligence.Channel{Provider: "cryon", Capability: "discovery"}):
			tracks, err = e.AutoMix(ctx, MixWeekly, step.Limit)
		case (intelligence.Channel{Provider: "cryon", Capability: "catalog_search"}):
			if strings.Contains(step.Query, "://") {
				return nil, nil
			}
			tracks, err = e.search(ctx, step.Query)
		case (intelligence.Channel{Provider: "lastfm", Capability: "similar_tracks"}):
			artist := ""
			if len(seed.Artists) > 0 {
				artist = seed.Artists[0]
			}
			metadata, fetchErr := e.lastfm.SimilarTracks(ctx, artist, seed.Title, step.Limit)
			err = fetchErr
			candidates := []scoredCand{}
			for _, t := range metadata {
				candidates = append(candidates, scoredCand{artist: t.Artist, title: t.Name, score: 1})
			}
			tracks = e.resolveCandidates(ctx, candidates, e.buildTaste(ctx), step.Limit)
		case (intelligence.Channel{Provider: "deezer", Capability: "artist_tracks"}):
			got, fetchErr := e.deezer.TopArtistTracks(ctx, step.Query, step.Limit)
			err = fetchErr
			metadata := deezerTracksToLastfm(got)
			candidates := []scoredCand{}
			for _, t := range metadata {
				candidates = append(candidates, scoredCand{artist: t.Artist, title: t.Name, score: 1})
			}
			tracks = e.resolveCandidates(ctx, candidates, e.buildTaste(ctx), step.Limit)
		case (intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"}):
			if e.trackSimilar != nil {
				tracks, err = e.trackSimilar(ctx, seed)
			}
		}
	}
	tracks = e.Allowed(ctx, tracks)
	filtered := []domain.Track{}
	for _, t := range balanceDJSources(e.orderCandidates(tracks)) {
		if t.PlayableKind == "" || t.PlayableKind == domain.PlayableStream {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) > step.Limit {
		filtered = filtered[:step.Limit]
	}
	return filtered, err
}

// Prevent the first catalog in aggregated search from consuming the whole budget.
func balanceDJSources(tracks []domain.Track) []domain.Track {
	groups := map[domain.ServiceID][]domain.Track{}
	order := []domain.ServiceID{}
	for _, track := range tracks {
		if _, exists := groups[track.Service]; !exists {
			order = append(order, track.Service)
		}
		groups[track.Service] = append(groups[track.Service], track)
	}
	out := make([]domain.Track, 0, len(tracks))
	for index := 0; len(out) < len(tracks); index++ {
		for _, source := range order {
			if index < len(groups[source]) {
				out = append(out, groups[source][index])
			}
		}
	}
	return out
}

func (e *Engine) FinalizeDJ(ctx context.Context, tracks []domain.Track, exclude []domain.Track, limit int) []domain.Track {
	seen := map[string]bool{}
	for _, t := range exclude {
		seen[trackKey(t)] = true
	}
	out := []domain.Track{}
	for _, t := range e.Allowed(ctx, tracks) {
		key := trackKey(t)
		if !seen[key] && t.Service != domain.ServiceYouTube && (t.PlayableKind == "" || t.PlayableKind == domain.PlayableStream) {
			seen[key] = true
			out = append(out, t)
		}
	}
	return diversifyTracks(out, nil, limit, artistCap)
}

// DJUnfamiliarArtists is an observable discovery proxy against the full local
// profile, not a claim that the user has never heard an artist elsewhere.
func (e *Engine) DJUnfamiliarArtists(ctx context.Context, tracks []domain.Track) []bool {
	known := familiar(e.buildTaste(ctx))
	result := make([]bool, len(tracks))
	for i, track := range tracks {
		key := artistKey(track)
		result[i] = key != "" && !known[key]
	}
	return result
}

// IntelligenceTaste exposes only compact metadata from the existing profile.
// Its revision changes when the exported content changes, not just on likes.
func (e *Engine) IntelligenceTaste(ctx context.Context) intelligence.TasteProfile {
	out := intelligence.TasteProfile{Artists: []string{}, Tags: []intelligence.WeightedTag{}}
	if e == nil {
		return out
	}
	profile := e.buildTaste(ctx)
	for _, artist := range profile.artists {
		if len(out.Artists) == 20 {
			break
		}
		out.Artists = append(out.Artists, artist.name)
	}
	// Reuse locally stored genres only. This accessor never enriches over network.
	tags := map[string]float64{}
	addTags := func(tracks []domain.Track, weight float64) {
		for _, track := range tracks {
			if genre := strings.TrimSpace(strings.ToLower(track.Genre)); genre != "" {
				tags[genre] += weight
			}
		}
	}
	if e.favFn != nil {
		if tracks, err := e.favFn(ctx); err == nil {
			addTags(tracks, 2)
		}
	}
	if e.histFn != nil {
		if rows, err := e.histFn(ctx, 100); err == nil {
			for _, row := range rows {
				addTags([]domain.Track{row.Track}, 1)
			}
		}
	}
	for name, weight := range tags {
		out.Tags = append(out.Tags, intelligence.WeightedTag{Name: name, Weight: weight})
	}
	sort.Slice(out.Tags, func(i, j int) bool {
		if out.Tags[i].Weight == out.Tags[j].Weight {
			return out.Tags[i].Name < out.Tags[j].Name
		}
		return out.Tags[i].Weight > out.Tags[j].Weight
	})
	if len(out.Tags) > 15 {
		out.Tags = out.Tags[:15]
	}
	raw, _ := json.Marshal(struct {
		Artists          []string
		Tags             []intelligence.WeightedTag
		FeedbackRevision int
	}{out.Artists, out.Tags, profile.feedbackRev})
	digest := sha256.Sum256(raw)
	// Keep revisions exactly representable in JSON/JavaScript and model output.
	out.Revision = binary.BigEndian.Uint64(digest[:8]) & ((1 << 52) - 1)
	return out
}
