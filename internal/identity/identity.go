// Package identity groups source-specific tracks into canonical musical works
// and ranks their playback candidates according to explicit user preferences.
package identity

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"Cryon2/internal/domain"
)

// Preferences define deterministic candidate selection without requiring a
// network call or a recommendation model.
type Preferences struct {
	PreferLocal    bool
	PreferExplicit bool
	OfficialOnly   bool
	SourcePriority map[domain.ServiceID]int
}

// Group represents one canonical work and all source-specific playback choices.
type Group struct {
	Track      domain.CanonicalTrack      `json:"track"`
	Candidates []domain.PlaybackCandidate `json:"candidates"`
}

// GroupTracks deduplicates equivalent source tracks using normalized artist,
// title and a conservative duration tolerance. It intentionally keeps uncertain
// matches separate rather than merging different recordings.
func GroupTracks(tracks []domain.Track) []Group {
	groups := make([]Group, 0, len(tracks))
	for _, track := range tracks {
		candidate := candidateFor(track)
		matched := -1
		for index := range groups {
			if sameWork(groups[index].Track, candidate.Track) {
				matched = index
				break
			}
		}
		if matched == -1 {
			canonical := canonicalFor(track)
			candidate.CanonicalTrackID = canonical.ID
			groups = append(groups, Group{Track: canonical, Candidates: []domain.PlaybackCandidate{candidate}})
			continue
		}
		candidate.CanonicalTrackID = groups[matched].Track.ID
		candidate.IdentityConfidence = confidence(groups[matched].Track, track)
		groups[matched].Candidates = append(groups[matched].Candidates, candidate)
		if candidate.IdentityConfidence > groups[matched].Track.IdentityConfidence {
			groups[matched].Track.IdentityConfidence = candidate.IdentityConfidence
		}
	}
	return groups
}

// RankCandidates returns available candidates in selection order. The returned
// candidates include a user-visible SelectionReason for diagnostics and UI.
func RankCandidates(candidates []domain.PlaybackCandidate, preferences Preferences) []domain.PlaybackCandidate {
	out := make([]domain.PlaybackCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Availability {
			continue
		}
		if preferences.OfficialOnly && !candidate.Official {
			continue
		}
		candidate.SourcePriority += preferences.SourcePriority[candidate.Track.Service]
		candidate.SelectionReason = selectionReason(candidate, preferences)
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return candidateScore(out[i], preferences) > candidateScore(out[j], preferences)
	})
	return out
}

func canonicalFor(track domain.Track) domain.CanonicalTrack {
	artist := firstArtist(track)
	normalizedArtist := Normalize(artist)
	normalizedTitle := Normalize(track.Title)
	seed := normalizedArtist + "|" + normalizedTitle
	sum := sha1.Sum([]byte(seed))
	return domain.CanonicalTrack{
		ID:                 fmt.Sprintf("trk_%x", sum[:8]),
		Title:              track.Title,
		NormalizedTitle:    normalizedTitle,
		Artist:             artist,
		NormalizedArtist:   normalizedArtist,
		Album:              track.Album,
		DurationMs:         track.DurationMs,
		ArtworkURL:         track.ArtworkURL,
		IdentityConfidence: 1,
		MetadataSource:     track.Service,
	}
}

func candidateFor(track domain.Track) domain.PlaybackCandidate {
	return domain.PlaybackCandidate{
		ID:                 string(track.Service) + ":" + track.ID,
		Track:              track,
		IdentityConfidence: 1,
		Version:            detectVersion(track.Title),
		Availability:       track.PlayableKind != domain.PlayableExternal,
		Official:           track.Service != domain.ServiceSoundCloud,
	}
}

func sameWork(canonical domain.CanonicalTrack, track domain.Track) bool {
	if canonical.NormalizedArtist != Normalize(firstArtist(track)) || canonical.NormalizedTitle != Normalize(track.Title) {
		return false
	}
	if canonical.DurationMs <= 0 || track.DurationMs <= 0 {
		return true
	}
	delta := canonical.DurationMs - track.DurationMs
	if delta < 0 {
		delta = -delta
	}
	return delta <= 8_000
}

func confidence(canonical domain.CanonicalTrack, track domain.Track) float64 {
	if !sameWork(canonical, track) {
		return 0
	}
	if canonical.DurationMs <= 0 || track.DurationMs <= 0 {
		return 0.85
	}
	delta := canonical.DurationMs - track.DurationMs
	if delta < 0 {
		delta = -delta
	}
	return 1 - float64(delta)/80_000
}

func candidateScore(candidate domain.PlaybackCandidate, preferences Preferences) float64 {
	score := candidate.IdentityConfidence*100 + float64(candidate.SourcePriority)
	if candidate.Version == domain.VersionOriginal || candidate.Version == domain.VersionUnknown {
		score += 12
	}
	if candidate.Version == domain.VersionLive || candidate.Version == domain.VersionCover {
		score -= 25
	}
	if preferences.PreferLocal && candidate.Track.Service == domain.ServiceLocal {
		score += 30
	}
	if preferences.PreferExplicit && candidate.Explicit {
		score += 15
	}
	if candidate.Official {
		score += 5
	}
	return score
}

func selectionReason(candidate domain.PlaybackCandidate, preferences Preferences) string {
	if preferences.PreferLocal && candidate.Track.Service == domain.ServiceLocal {
		return "Выбрана локальная версия по настройке"
	}
	if preferences.PreferExplicit && candidate.Explicit {
		return "Выбрана explicit-версия по настройке"
	}
	if candidate.Version == domain.VersionOriginal || candidate.Version == domain.VersionUnknown {
		return "Выбрана наиболее точная доступная версия"
	}
	return "Выбрана доступная версия с лучшим совпадением"
}

func detectVersion(title string) domain.VersionKind {
	normalized := Normalize(title)
	switch {
	case strings.Contains(normalized, "live"):
		return domain.VersionLive
	case strings.Contains(normalized, "cover"):
		return domain.VersionCover
	case strings.Contains(normalized, "remaster") || strings.Contains(normalized, "remastered"):
		return domain.VersionRemaster
	case strings.Contains(normalized, "clean"):
		return domain.VersionClean
	default:
		return domain.VersionOriginal
	}
}

func firstArtist(track domain.Track) string {
	if len(track.Artists) == 0 {
		return ""
	}
	return track.Artists[0]
}

// Normalize creates a stable comparison key while preserving all letters and
// digits, including Cyrillic. Featured-artist suffixes and punctuation do not
// change canonical identity.
func Normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.Index(value, " feat."); index >= 0 {
		value = value[:index]
	}
	if index := strings.Index(value, " ft."); index >= 0 {
		value = value[:index]
	}
	for _, suffix := range []string{"official audio", "official video", "audio only", "lyrics video"} {
		value = strings.ReplaceAll(value, suffix, "")
	}
	var out strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			out.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(out.String())
}
