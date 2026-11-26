package intelligence

import "testing"

func TestPersonalSearchNeedsNoUserQueryOrModel(t *testing.T) {
	in, limits := fixture()
	in.Session.Mode = RadioInfinite
	in.Channels = append(in.Channels, ChannelBudget{Channel: Channel{Provider: "cryon", Capability: "discovery"}, MaxCandidates: 40})
	p := PersonalSearch(in)
	if err := ValidateSearch(in, p, limits); err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Channel.Capability != "discovery" {
		t.Fatal(p)
	}
	in.Session.Mode = RadioTrack
	p = PersonalSearch(in)
	if err := ValidateSearch(in, p, limits); err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Channel.Capability != "similar_tracks" {
		t.Fatal(p)
	}
}

func TestPersonalSelectionGraphNoveltyBlocksAndSeed(t *testing.T) {
	in, limits := fixture()
	in.Session.Mode = RadioInfinite
	in.Taste.BlockedArtists = []string{"Blocked"}
	good := Candidate{ID: "new", UnfamiliarArtist: true, Track: TrackSummary{Artists: []string{"New artist"}}, Evidence: Evidence{MetadataQuality: .9}, Channels: []Channel{{Provider: "cryon", Capability: "discovery"}}}
	played := good
	played.ID = "played"
	played.Evidence.RecentlyPlayed = true
	blocked := good
	blocked.ID = "blocked"
	blocked.Track.Artists = []string{"Blocked"}
	random := good
	random.ID = "random"
	random.Channels = nil
	sel := SelectionInput{Context: in, PoolID: "pool", Candidates: []Candidate{good, played, blocked, random}}
	p := PersonalSelection(sel)
	if err := ValidateSelection(sel, p, limits); err != nil {
		t.Fatal(err)
	}
	if len(p.Ranked) != 1 || p.Ranked[0].ID != "new" {
		t.Fatal(p)
	}
	sel.Context.Session.Mode = RadioTrack
	if got := PersonalSelection(sel); len(got.Ranked) != 0 {
		t.Fatal("global graph replaced seed", got)
	}
	sel.Candidates[0].Channels = []Channel{{Provider: "lastfm", Capability: "similar_tracks"}}
	if got := PersonalSelection(sel); len(got.Ranked) != 1 {
		t.Fatal(got)
	}
}
