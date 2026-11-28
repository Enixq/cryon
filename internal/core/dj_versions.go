package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/services/vkweb"
)

// A queue segment has at most 12 works, 60 source queries, 8 concurrent
// operations and no retries. Existing positive source caches are reused.
// Late/uncooperative adapters keep their slot until they actually return.
const djVersionLimit = 12
const djVersionConcurrency = 8

type djVersionResult struct {
	index, source int
	tracks        []domain.Track
	err           error
	elapsed       time.Duration
	attempted     bool
	cached        bool
}

func (a *App) djVersionSlots() chan struct{} {
	a.dj.Lock()
	defer a.dj.Unlock()
	if a.dj.versionSlots == nil {
		a.dj.versionSlots = make(chan struct{}, djVersionConcurrency)
	}
	return a.dj.versionSlots
}

func (a *App) resolveDJVersions(ctx context.Context, ordered []domain.Track, candidates []intelligence.Candidate, diagnostic *DJDiagnostic) ([]domain.Track, []intelligence.Candidate) {
	// Minimal engines used without a service registry retain their existing
	// behavior. Production Windows and gomobile both use NewApp's registry.
	if a.registry == nil {
		return ordered, candidates
	}
	if len(ordered) > djVersionLimit {
		ordered = ordered[:djVersionLimit]
	}
	policy := a.djSourcePolicy(ctx)
	diagnostic.Stage = "resolving"
	diagnostic.Sources = a.djSourceDiagnostics(policy)
	a.saveDJDiagnostic(ctx, *diagnostic)
	searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	results := make(chan djVersionResult, len(ordered)*len(djVersionSources))
	slots := a.djVersionSlots()
	cacheScope := a.sourceCacheScope(ctx)
	queries := make([]atomic.Int32, len(djVersionSources))
	completed := make([]int, len(djVersionSources))
	pending := 0
	for s, status := range diagnostic.Sources {
		if status.State != "ready" {
			continue
		}
		pending += len(ordered)
		diagnostic.Sources[s].SearchAttempted = len(ordered) > 0
		// VK's existing website adapter serializes requests and enforces a
		// three-second search interval. Never fill global slots waiting for its
		// internal gate, and give the actual call time to include that interval.
		workers := 2
		if status.Source == domain.ServiceVK || status.Source == domain.ServiceLocal {
			workers = 1
		}
		for worker := 0; worker < workers; worker++ {
			go func(s, worker, workers int) {
				for i := worker; i < len(ordered); i += workers {
					r := djVersionResult{index: i, source: s}
					if searchCtx.Err() != nil {
						r.err = searchCtx.Err()
						results <- r
						continue
					}
					select {
					case slots <- struct{}{}:
					case <-searchCtx.Done():
						r.err = searchCtx.Err()
						results <- r
						continue
					}
					if searchCtx.Err() != nil {
						<-slots
						r.err = searchCtx.Err()
						results <- r
						continue
					}
					started := time.Now()
					timeout := 3 * time.Second
					if djVersionSources[s] == domain.ServiceVK {
						timeout = 5 * time.Second
					}
					callCtx, stop := context.WithTimeout(searchCtx, timeout)
					queries[s].Add(1)
					query := ordered[i].Title
					// Local.Search matches each metadata field separately; an
					// artist+title string would never match a normal local item.
					if djVersionSources[s] != domain.ServiceLocal && len(ordered[i].Artists) > 0 {
						query = ordered[i].Artists[0] + " " + query
					}
					r.tracks, r.cached, r.err = a.searchDJVersions(callCtx, a.registry[djVersionSources[s]], query, cacheScope)
					r.attempted = true
					stop()
					<-slots
					r.elapsed = time.Since(started)
					if len(r.tracks) > 20 {
						r.tracks = r.tracks[:20]
					}
					results <- r
				}
			}(s, worker, workers)
		}
	}
	versions := make([][]domain.PlaybackCandidate, len(ordered))
	canonical := make([]string, len(ordered))
	for i, track := range ordered {
		canonical[i] = "recording:" + identity.Normalize(strings.Join(track.Artists, " ")) + ":" + identity.Normalize(track.Title)
		for _, candidate := range candidates {
			if candidate.Track.ID == djTrackID(track) && candidate.CanonicalID != "" {
				canonical[i] = candidate.CanonicalID
				break
			}
		}
	}
	lastReport := time.Now()
	for pending > 0 {
		var r djVersionResult
		select {
		case r = <-results:
			pending--
		case <-searchCtx.Done():
			// Preserve completed rows even when the last slow request expires.
			select {
			case r = <-results:
				pending--
			default:
				for s := range diagnostic.Sources {
					if diagnostic.Sources[s].SearchAttempted {
						diagnostic.Sources[s].Reasons["search_budget_expired"]++
					}
				}
				pending = 0
				continue
			}
		}
		if time.Since(lastReport) > 400*time.Millisecond {
			a.saveDJDiagnostic(ctx, *diagnostic)
			lastReport = time.Now()
		}
		d := &diagnostic.Sources[r.source]
		completed[r.source]++
		d.ElapsedMS += r.elapsed.Milliseconds()
		if r.attempted {
			d.Queries++
		}
		if r.cached {
			d.Cached++
		}
		if r.err != nil {
			d.Reasons["search_error"]++
			if errors.Is(r.err, context.DeadlineExceeded) || errors.Is(r.err, context.Canceled) {
				d.Reasons["search_timeout"]++
			}
			var vkError *vkweb.Error
			if errors.As(r.err, &vkError) {
				if vkError.Kind == "session" {
					d.Auth = "expired"
					d.State = "no_auth"
				}
				if vkError.HTTP == 429 {
					d.Reasons["rate_limited"]++
				}
			}
		} else if r.attempted && !r.cached && d.Auth == "session_present" {
			d.Auth = "verified"
		}
		d.Found += len(r.tracks)
		for _, track := range r.tracks {
			if track.Service != d.Source || track.ID == "" {
				d.Rejected++
				d.Reasons["invalid_source_id"]++
				continue
			}
			score, reason := identity.MatchRecording(ordered[r.index], track)
			if score == 0 {
				d.Rejected++
				d.Reasons[reason]++
				continue
			}
			d.Matched++
			pc := domain.PlaybackCandidate{ID: djTrackID(track), CanonicalTrackID: canonical[r.index], Track: track, IdentityConfidence: score, Version: identity.RecordingVersion(ordered[r.index].Title), AvailabilityState: "unchecked"}
			if track.PlayableKind != "" && track.PlayableKind != domain.PlayableStream {
				pc.AvailabilityState = "unplayable"
				pc.RejectionReason = "no_direct_stream"
				d.Rejected++
				d.Reasons[pc.RejectionReason]++
			}
			versions[r.index] = append(versions[r.index], pc)
		}
	}
	cancel()
	for s := range diagnostic.Sources {
		d := &diagnostic.Sources[s]
		d.Queries = int(queries[s].Load())
		if d.SearchAttempted && completed[s] < len(ordered) {
			d.Reasons["search_budget_expired"] = len(ordered) - completed[s]
		} else {
			delete(d.Reasons, "search_budget_expired")
		}
		d.SearchAttempted = d.Queries > 0
	}
	a.saveDJDiagnostic(ctx, *diagnostic)
	// Preserve a known source item if catalog search is empty, but still check
	// its stream. It is never reported as a search hit or an identity match.
	for i, track := range ordered {
		for _, d := range diagnostic.Sources {
			if d.Source == track.Service && d.Enabled && d.State == "ready" {
				versions[i] = append(versions[i], domain.PlaybackCandidate{ID: djTrackID(track), CanonicalTrackID: canonical[i], Track: track, IdentityConfidence: 1, Version: identity.RecordingVersion(track.Title), AvailabilityState: "unchecked"})
			}
		}
		seen := map[string]bool{}
		unique := []domain.PlaybackCandidate{}
		for _, pc := range versions[i] {
			if !seen[pc.ID] {
				seen[pc.ID] = true
				unique = append(unique, pc)
			}
		}
		versions[i] = unique
	}
	// Plan soft diversity deterministically before asynchronous stream checks.
	counts := map[domain.ServiceID]int{}
	for i := range versions {
		sort.SliceStable(versions[i], func(x, y int) bool {
			left, right := djVersionScore(versions[i][x], policy, counts), djVersionScore(versions[i][y], policy, counts)
			if left == right {
				return versions[i][x].ID < versions[i][y].ID
			}
			return left > right
		})
		if len(versions[i]) > 0 {
			counts[versions[i][0].Track.Service]++
		}
	}
	probeCtx, stop := context.WithTimeout(ctx, 12*time.Second)
	defer stop()
	type checked struct {
		index    int
		versions []domain.PlaybackCandidate
		picked   int
	}
	checks := make(chan checked, len(ordered))
	probeBudget := make(chan struct{}, 48)
	for i := range ordered {
		rows := append([]domain.PlaybackCandidate{}, versions[i]...)
		go func(i int, rows []domain.PlaybackCandidate) {
			r := checked{index: i, versions: rows, picked: -1}
			defer func() { checks <- r }()
			for j := range r.versions {
				pc := &r.versions[j]
				if pc.AvailabilityState == "unplayable" {
					continue
				}
				select {
				case slots <- struct{}{}:
				case <-probeCtx.Done():
					return
				}
				select {
				case probeBudget <- struct{}{}:
				default:
					<-slots
					return
				}
				timeout := 3500 * time.Millisecond
				if pc.Track.Service == domain.ServiceYouTube {
					timeout = 4 * time.Second
				}
				started := time.Now()
				callCtx, end := context.WithTimeout(probeCtx, timeout)
				err := a.probeDJVersion(callCtx, pc.Track)
				end()
				pc.CheckedAt = time.Now().UTC().Format(time.RFC3339)
				pc.CheckElapsedMS = time.Since(started).Milliseconds()
				<-slots
				if err != nil {
					pc.AvailabilityState = "unplayable"
					pc.RejectionReason = "stream_unavailable"
				} else {
					pc.Availability = true
					pc.AvailabilityState = "playable"
					pc.SelectionReason = "identity_stream_priority_diversity"
					r.picked = j
					return
				}
				if probeCtx.Err() != nil {
					return
				}
			}
		}(i, rows)
	}
	checkedRows := make(map[int]checked)
	for n := 0; n < len(ordered); n++ {
		select {
		case r := <-checks:
			checkedRows[r.index] = r
		case <-probeCtx.Done():
			n = len(ordered)
		}
	}
	// A deadline and completed result can become readable together.
drainChecks:
	for {
		select {
		case r := <-checks:
			checkedRows[r.index] = r
		default:
			break drainChecks
		}
	}
	out := []domain.Track{}
	for i, anchor := range ordered {
		r, ok := checkedRows[i]
		if ok {
			versions[i] = r.versions
		}
		for _, pc := range versions[i] {
			for s := range diagnostic.Sources {
				d := &diagnostic.Sources[s]
				if d.Source != pc.Track.Service {
					continue
				}
				d.ElapsedMS += pc.CheckElapsedMS
				if pc.AvailabilityState == "unchecked" {
					d.Unchecked++
					if !ok || (r.picked < 0 && (probeCtx.Err() != nil || len(probeBudget) == cap(probeBudget))) {
						d.Reasons["probe_budget_expired"]++
					}
				}
				if pc.AvailabilityState == "playable" {
					d.Playable++
				}
				if pc.RejectionReason == "stream_unavailable" {
					d.Rejected++
					d.Reasons[pc.RejectionReason]++
				}
			}
		}
		if ok && r.picked >= 0 {
			track := versions[i][r.picked].Track
			// Keep musical metadata and feedback identity independent of uploader
			// presentation. Raw source metadata remains in PlaybackCandidates.
			track.Title, track.Artists = anchor.Title, append([]string{}, anchor.Artists...)
			track.DJSessionID, track.DJPlanID, track.RecommendationReason = anchor.DJSessionID, anchor.DJPlanID, anchor.RecommendationReason
			out = append(out, track)
			for s := range diagnostic.Sources {
				if diagnostic.Sources[s].Source == track.Service {
					diagnostic.Sources[s].Chosen++
				}
			}
		}
		for j := range candidates {
			if candidates[j].Track.ID != djTrackID(anchor) {
				continue
			}
			candidates[j].CanonicalID = canonical[i]
			candidates[j].PlaybackCandidates = versions[i]
			if ok && r.picked >= 0 {
				candidates[j].Track = djSummary(out[len(out)-1])
			}
		}
	}
	for s := range diagnostic.Sources {
		d := &diagnostic.Sources[s]
		if d.State != "ready" {
			continue
		}
		switch {
		case d.Chosen > 0:
			d.State = "selected"
		case d.Playable > 0:
			d.State = "playable"
		case d.Unchecked > 0:
			d.State = "matched_unchecked"
		case d.Reasons["search_error"] > 0:
			d.State = "search_error"
		case d.Found == 0:
			d.State = "empty"
		case d.Matched == 0:
			d.State = "identity_rejected"
		case d.Reasons["stream_unavailable"] > 0 || d.Reasons["no_direct_stream"] > 0:
			d.State = "unplayable"
		default:
			d.State = "matched_unchecked"
		}
	}
	diagnostic.Selected = len(out)
	if len(ordered) > 0 && len(out) == 0 {
		diagnostic.Error = "Музыка отобрана, но источники не подтвердили доступные потоки; подробности по каждому источнику ниже"
	}
	a.saveDJDiagnostic(ctx, *diagnostic)
	return out, candidates
}

func djVersionScore(pc domain.PlaybackCandidate, p DJSourcePolicy, counts map[domain.ServiceID]int) float64 {
	score := pc.IdentityConfidence*100 - float64(min(counts[pc.Track.Service], 6))*2
	if pc.Track.Service == domain.ServiceLocal {
		score += 3
	}
	for i, id := range p.Priority {
		if id == pc.Track.Service {
			score += float64(len(p.Priority)-i) * 20
			break
		}
	}
	return score
}

func (a *App) probeDJVersion(ctx context.Context, track domain.Track) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if a.djVersionProbe != nil {
		return a.djVersionProbe(ctx, track)
	}
	stream, err := a.getAudioStream(ctx, string(track.Service), track.ID)
	if err != nil {
		return err
	}
	if stream == nil || stream.URL == "" {
		return fmt.Errorf("empty stream")
	}
	if track.Service == domain.ServiceLocal || (!strings.HasPrefix(stream.URL, "https://") && !strings.HasPrefix(stream.URL, "http://")) {
		file, err := os.Stat(stream.URL)
		if err == nil && (file.IsDir() || file.Size() == 0) {
			return fmt.Errorf("empty local audio")
		}
		return err
	}
	if track.Service == domain.ServiceYouTube || track.Service == domain.ServiceSoundCloud {
		return nil
	} // Existing resolver probes googlevideo.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if track.Service == domain.ServiceVK {
		req.Header.Set("Referer", "https://vk.ru/audio")
		req.Header.Set("Origin", "https://vk.ru")
	}
	response, err := streamDownloadClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 || strings.Contains(response.Header.Get("Content-Type"), "text/html") {
		return fmt.Errorf("stream HTTP %d", response.StatusCode)
	}
	_, err = io.CopyN(io.Discard, response.Body, 1)
	return err
}

// Stored DJ alternatives are identity-checked versions of the same recording.
// Never fall through to the general title-only fallback for a DJ track.
func (a *App) djPlaybackFallback(track domain.Track, failed string, excluded []string) (domain.PlaybackCandidate, bool) {
	repo, ok := a.store.(intelligence.SessionRepository)
	if !ok {
		return domain.PlaybackCandidate{}, false
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	candidates, err := repo.DJCandidates(ctx, track.DJSessionID, track.DJPlanID)
	if err != nil {
		return domain.PlaybackCandidate{}, false
	}
	policy := a.djSourcePolicy(ctx)
	blocked := map[string]bool{djTrackID(track): true}
	for _, id := range excluded {
		blocked[id] = true
	}
	for _, candidate := range candidates {
		belongs := candidate.Track.ID == djTrackID(track)
		for _, pc := range candidate.PlaybackCandidates {
			if pc.ID == djTrackID(track) {
				belongs = true
			}
		}
		if !belongs {
			continue
		}
		versions := append([]domain.PlaybackCandidate{}, candidate.PlaybackCandidates...)
		sort.SliceStable(versions, func(i, j int) bool {
			return djVersionScore(versions[i], policy, nil) > djVersionScore(versions[j], policy, nil)
		})
		for _, pc := range versions {
			if blocked[pc.ID] || string(pc.Track.Service) == failed || !policy.Enabled[pc.Track.Service] || pc.AvailabilityState == "unplayable" {
				continue
			}
			timeout := 3500 * time.Millisecond
			if pc.Track.Service == domain.ServiceYouTube {
				timeout = 4 * time.Second
			}
			check, end := context.WithTimeout(ctx, timeout)
			err := a.probeDJVersion(check, pc.Track)
			end()
			if err != nil {
				continue
			}
			pc.Availability = true
			pc.AvailabilityState = "playable"
			pc.SelectionReason = "same_recording_fallback"
			pc.Track.Title, pc.Track.Artists = candidate.Track.Title, append([]string{}, candidate.Track.Artists...)
			pc.Track.DJSessionID, pc.Track.DJPlanID, pc.Track.RecommendationReason = track.DJSessionID, track.DJPlanID, track.RecommendationReason
			return pc, true
		}
	}
	return domain.PlaybackCandidate{}, false
}

// Reuse positive search snapshots, but preserve source errors instead of
// silently reviving stale rows. Empty successful searches are cached briefly.
func (a *App) searchDJVersions(ctx context.Context, svc domain.MusicService, query, scope string) ([]domain.Track, bool, error) {
	if svc.ID() == domain.ServiceLocal {
		tracks, err := svc.Search(ctx, query)
		return tracks, false, err
	}
	version := "v2"
	if svc.ID() == domain.ServiceVK {
		version = "vk-site-v1"
	}
	key := fmt.Sprintf("search:%s:%s:%s:%x", version, svc.ID(), scope, sha256.Sum256([]byte(query)))
	if a.store != nil {
		if raw, ok, _ := a.store.RecoCacheGet(ctx, key); ok {
			var tracks []domain.Track
			if json.Unmarshal([]byte(raw), &tracks) == nil {
				return tracks, true, nil
			}
		}
	}
	tracks, err := svc.Search(ctx, query)
	if err == nil && a.store != nil {
		ttl := 6 * time.Hour
		if len(tracks) == 0 {
			ttl = 45 * time.Second
		}
		raw, _ := json.Marshal(tracks)
		_ = a.store.RecoCacheSet(ctx, key, string(raw), ttl)
	}
	return tracks, false, err
}
