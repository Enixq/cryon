package identity

import (
	"math"
	"strings"
	"unicode"

	"Cryon2/internal/domain"
)

// MatchRecording is deliberately independent of source preference. A matching
// title alone is never evidence of a matching recording. Presentation suffixes
// on video platforms are tolerated; recording variants are not interchangeable.
func MatchRecording(anchor, found domain.Track) (float64, string) {
	title := recordingTitle(anchor.Title)
	otherTitle := found.Title
	other := recordingTitle(found.Title)
	artistMatch := false
	for _, artist := range anchor.Artists {
		a := artistName(artist)
		if a == "" {
			continue
		}
		for _, candidate := range found.Artists {
			if a == artistName(candidate) {
				artistMatch = true
			}
		}
		// YouTube/SoundCloud often put the recording artist in the title and
		// the uploading channel in Artists. Require both artist and title.
		for _, separator := range []string{" - ", " – ", " — "} {
			parts := strings.SplitN(found.Title, separator, 2)
			if len(parts) == 2 && artistName(parts[0]) == a {
				otherTitle = parts[1]
				other = recordingTitle(parts[1])
				artistMatch = true
			}
		}
	}
	if !artistMatch {
		return 0, "artist_mismatch"
	}
	if recordingVariant(anchor.Title) != recordingVariant(otherTitle) {
		return 0, "recording_variant"
	}
	if title == "" || title != other {
		return 0, "title_mismatch"
	}
	score := .90
	if anchor.DurationMs > 0 && found.DurationMs > 0 {
		difference := math.Abs(float64(anchor.DurationMs - found.DurationMs))
		if difference > math.Max(8000, float64(anchor.DurationMs)*.04) {
			return 0, "duration_mismatch"
		}
		score += .07 * (1 - difference/math.Max(8000, float64(anchor.DurationMs)*.04))
	}
	if anchor.Album != "" && found.Album != "" && Normalize(anchor.Album) == Normalize(found.Album) {
		score += .03
	}
	return math.Min(1, score), "matched"
}

func RecordingVersion(title string) domain.VersionKind {
	if strings.Contains(recordingVariant(title), "acoustic") || strings.Contains(recordingVariant(title), "акустика") {
		return domain.VersionAcoustic
	}
	return detectVersion(title)
}

func recordingTitle(title string) string {
	title = strings.ToLower(title)
	for _, decoration := range []string{"official music video", "official lyric video", "official lyrics video", "official audio", "official video", "audio only", "lyrics video", "[lyrics]", "(lyrics)", "[hd]", "(hd)", "[4k]", "(4k)"} {
		title = strings.ReplaceAll(title, decoration, "")
	}
	return Normalize(title)
}

func artistName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{" - topic", " – topic", "vevo", " official"} {
		name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
	}
	return Normalize(name)
}

func recordingVariant(title string) string {
	// Do not use Normalize here: its featured-artist truncation could hide
	// a '(Live)' suffix after 'feat.'.
	tokens := " " + strings.Join(strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ") + " "
	var variants []string
	for _, word := range []string{"live", "concert", "remix", "acoustic", "акустика", "remaster", "remastered", "cover", "кавер", "instrumental", "karaoke", "sped", "slowed", "clean"} {
		if strings.Contains(tokens, " "+word+" ") {
			variants = append(variants, word)
		}
	}
	return strings.Join(variants, ":")
}
