// Package vkweb uses the authenticated VK website's audio player protocol.
package vkweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"golang.org/x/net/html/charset"
	"golang.org/x/net/publicsuffix"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var chromeVersion = regexp.MustCompile(`Chrome/([0-9.]+)`)

// al_audio.php is the desktop player protocol. A mobile WebView identity sends
// /audio to m.vk.ru, whose session and page state are different.
func websiteUserAgent(ua string) string {
	if ua == "" {
		return userAgent
	}
	lower := strings.ToLower(ua)
	if !strings.Contains(lower, "android") && !strings.Contains(lower, "mobile") && !strings.Contains(lower, "; wv") {
		return ua
	}
	if version := chromeVersion.FindStringSubmatch(ua); len(version) == 2 {
		return strings.Replace(userAgent, "Chrome/131.0.0.0", "Chrome/"+version[1], 1)
	}
	return userAgent
}

type Session struct {
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
}

func (s Session) Valid() bool {
	if len(s.Cookies) > 32768 || len(s.UserAgent) > 1024 || strings.ContainsAny(s.Cookies+s.UserAgent, "\r\n") {
		return false
	}
	r := &http.Request{Header: http.Header{"Cookie": {s.Cookies}}}
	for _, c := range r.Cookies() {
		if (c.Name == "remixsid" || c.Name == "remixnsid") && c.Value != "" {
			return true
		}
	}
	return false
}

// Error exposes local labels and status codes, never server bodies or credentials.
type Error struct {
	Stage, Kind string
	Code, HTTP  int
	RetryAfter  time.Duration
}

func (e *Error) Error() string {
	label := map[string]string{"website": "проверка сессии сайта", "section": "поиск музыки", "reload_audio": "получение трека", "browser": "открытие браузера"}[e.Stage]
	if label == "" {
		label = "подключение ВК"
	}
	var reason string
	switch {
	case e.Kind == "session":
		reason = "сессия сайта ВК истекла; войдите через браузер заново"
	case e.Kind == "browser_missing":
		reason = "не найден Edge или Chrome"
	case e.Kind == "browser_closed":
		reason = "окно входа закрыто"
	case e.Kind == "browser_timeout":
		reason = "браузер не ответил вовремя; повторите вход"
	case e.Kind == "legacy":
		reason = "трек сохранён через прежний API; найдите его заново"
	case e.Kind == "restricted":
		reason = "ВК не предоставил доступный аудиопоток"
	case e.HTTP == http.StatusTooManyRequests:
		reason = "сайт ВК временно ограничил запросы"
		if e.RetryAfter > 0 {
			reason += fmt.Sprintf("; повторите через %d сек", int(e.RetryAfter.Seconds())+1)
		}
	case e.HTTP != 0:
		reason = fmt.Sprintf("ВК ответил HTTP %d", e.HTTP)
	case e.Code != 0:
		reason = fmt.Sprintf("сайт ВК отклонил запрос (код %d); откройте сайт и проверьте уведомления", e.Code)
	case e.Kind == "json":
		reason = "формат ответа веб-плеера ВК изменился или сайт запросил проверку безопасности"
	case e.Kind == "network":
		reason = "не удалось установить соединение с ВК"
	default:
		reason = "не удалось завершить этот этап"
	}
	return "ВК: " + label + " — " + reason + "."
}

type cachedSearch struct {
	tracks  []domain.Track
	expires time.Time
}
type cachedStream struct {
	target, mime string
	expires      time.Time
}
type Service struct {
	mu         sync.RWMutex
	session    Session
	generation uint64
	http       *http.Client
	siteBase   string
	gate       chan struct{}
	// These fields are used only while holding gate.
	activeGen        uint64
	uid              int64
	next, searchNext time.Time
	searches         map[string]cachedSearch
	streams          map[string]cachedStream
	candidates       map[string]audio
	// cooldownUntil is also read by the UI, under mu.
	cooldownUntil time.Time
}

func New() *Service {
	s := &Service{siteBase: "https://vk.ru", gate: make(chan struct{}, 1)}
	s.SetSession(Session{})
	return s
}
func (s *Service) ID() domain.ServiceID { return domain.ServiceVK }
func (s *Service) HasToken() bool       { s.mu.RLock(); defer s.mu.RUnlock(); return s.session.Valid() }

func allowedRedirect(req *http.Request, via []*http.Request) error {
	host := req.URL.Hostname()
	if len(via) >= 8 || req.URL.Scheme != "https" || req.URL.User != nil || !(host == "vk.ru" || strings.HasSuffix(host, ".vk.ru") || host == "vk.com" || strings.HasSuffix(host, ".vk.com")) {
		return http.ErrUseLastResponse
	}
	return nil
}
func (s *Service) SetSession(session Session) {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if session.Valid() {
		session.UserAgent = websiteUserAgent(session.UserAgent)
		for _, root := range []string{"https://vk.com", s.siteBase} {
			target, err := url.Parse(root)
			if err != nil {
				continue
			}
			r := &http.Request{Header: http.Header{"Cookie": {session.Cookies}}}
			cookies := r.Cookies()
			for _, c := range cookies {
				c.Domain = target.Hostname()
				c.Path = "/"
				c.Secure = target.Scheme == "https"
			}
			jar.SetCookies(target, cookies)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.generation++
	s.http = &http.Client{Timeout: 20 * time.Second, Jar: jar, CheckRedirect: allowedRedirect}
}

// Session returns cookies updated by the site's login redirects for persistence.
func (s *Service) Session() Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session := s.session
	var cookies []BrowserCookie
	for _, root := range []string{"https://vk.com", s.siteBase} {
		u, err := url.Parse(root)
		if err != nil {
			continue
		}
		for _, c := range s.http.Jar.Cookies(u) {
			cookies = append(cookies, BrowserCookie{Name: c.Name, Value: c.Value, Domain: "vk.ru"})
		}
	}
	if len(cookies) > 0 {
		session.Cookies = FormatCookies(cookies)
	}
	return session
}
func (s *Service) RateLimit() *Error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if remaining := time.Until(s.cooldownUntil); remaining > 0 {
		return &Error{HTTP: 429, RetryAfter: remaining}
	}
	return nil
}

func (s *Service) acquire(ctx context.Context, stage string) (*http.Client, Session, uint64, error) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, Session{}, 0, ctx.Err()
	}
	s.mu.RLock()
	client, session, gen := s.http, s.session, s.generation
	s.mu.RUnlock()
	if ctx.Err() != nil || !session.Valid() {
		<-s.gate
		if ctx.Err() != nil {
			return nil, session, gen, ctx.Err()
		}
		return nil, session, gen, &Error{Stage: stage, Kind: "session"}
	}
	if s.activeGen != gen {
		s.activeGen = gen
		s.uid = 0
		s.searches = make(map[string]cachedSearch)
		s.streams = make(map[string]cachedStream)
		s.candidates = make(map[string]audio)
	}
	return client, session, gen, nil
}
func (s *Service) unchanged(gen uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation == gen
}

func (s *Service) fetch(ctx context.Context, client *http.Client, session Session, target string, values url.Values, stage string) ([]byte, error) {
	if limited := s.RateLimit(); limited != nil {
		limited.Stage = stage
		return nil, limited
	}
	next := s.next
	if stage == "section" && s.searchNext.After(next) {
		next = s.searchNext
	}
	if delay := time.Until(next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	method := http.MethodGet
	var body io.Reader
	if values != nil {
		method = http.MethodPost
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, &Error{Stage: stage, Kind: "network"}
	}
	ua := session.UserAgent
	if ua == "" {
		ua = userAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", s.siteBase+"/audio")
	if values != nil {
		req.Header.Set("Origin", s.siteBase)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	s.next = time.Now().Add(1100 * time.Millisecond)
	if stage == "section" {
		s.searchNext = time.Now().Add(3 * time.Second)
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Stage: stage, Kind: "network"}
	}
	defer res.Body.Close()
	if res.StatusCode == 429 {
		pause := 30 * time.Second
		if seconds, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && seconds > 0 {
			pause = time.Duration(seconds) * time.Second
		} else if date, e := http.ParseTime(res.Header.Get("Retry-After")); e == nil && time.Until(date) > 0 {
			pause = time.Until(date)
		}
		if pause > 15*time.Minute {
			pause = 15 * time.Minute
		}
		s.mu.Lock()
		s.cooldownUntil = time.Now().Add(pause)
		s.mu.Unlock()
		return nil, &Error{Stage: stage, HTTP: 429, RetryAfter: pause}
	}
	if res.StatusCode != 200 {
		return nil, &Error{Stage: stage, HTTP: res.StatusCode}
	}
	reader, err := charset.NewReader(res.Body, res.Header.Get("Content-Type"))
	if err != nil {
		return nil, &Error{Stage: stage, Kind: "json"}
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		return nil, &Error{Stage: stage, Kind: "network"}
	}
	return raw, nil
}

var websiteUID = regexp.MustCompile(`(?:"uid"\s*:\s*|(?:window\.)?vk\s*=\s*\{\s*"id"\s*:\s*)(\d+)`)

func (s *Service) ensureUser(ctx context.Context, client *http.Client, session Session) error {
	if s.uid != 0 {
		return nil
	}
	body, err := s.fetch(ctx, client, session, s.siteBase+"/audio", nil, "website")
	if err != nil {
		return err
	}
	match := websiteUID.FindSubmatch(body)
	if len(match) != 2 {
		return &Error{Stage: "website", Kind: "session"}
	}
	s.uid, _ = strconv.ParseInt(string(match[1]), 10, 64)
	if s.uid <= 0 {
		return &Error{Stage: "website", Kind: "session"}
	}
	return nil
}
func (s *Service) alAudio(ctx context.Context, client *http.Client, session Session, values url.Values) ([]json.RawMessage, error) {
	stage := values.Get("act")
	body, err := s.fetch(ctx, client, session, s.siteBase+"/al_audio.php?act="+url.QueryEscape(stage), values, stage)
	if err != nil {
		return nil, err
	}
	var response struct {
		Payload []json.RawMessage `json:"payload"`
	}
	body = []byte(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(body)), "<!--")))
	if json.Unmarshal(body, &response) != nil || len(response.Payload) < 2 {
		return nil, &Error{Stage: stage, Kind: "json"}
	}
	var code int
	if json.Unmarshal(response.Payload[0], &code) != nil {
		var text string
		if json.Unmarshal(response.Payload[0], &text) != nil {
			return nil, &Error{Stage: stage, Kind: "json"}
		}
		var e error
		code, e = strconv.Atoi(text)
		if e != nil {
			return nil, &Error{Stage: stage, Kind: "json"}
		}
	}
	if code != 0 {
		kind := ""
		if code == 3 {
			kind = "session"
			s.uid = 0
		}
		return nil, &Error{Stage: stage, Kind: kind, Code: code}
	}
	var inner []json.RawMessage
	if json.Unmarshal(response.Payload[1], &inner) != nil {
		return nil, &Error{Stage: stage, Kind: "json"}
	}
	return inner, nil
}
func (s *Service) ValidateToken(ctx context.Context) error {
	client, session, gen, err := s.acquire(ctx, "website")
	if err != nil {
		return err
	}
	defer func() { <-s.gate }()
	// Explicit validation checks the site again, rather than trusting a cached uid.
	s.uid = 0
	err = s.ensureUser(ctx, client, session)
	if !s.unchanged(gen) {
		return &Error{Stage: "website", Kind: "session"}
	}
	return err
}
func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	query = strings.TrimSpace(query)
	if !s.HasToken() || query == "" {
		return []domain.Track{}, nil
	}
	client, session, gen, err := s.acquire(ctx, "section")
	if err != nil {
		return nil, err
	}
	defer func() { <-s.gate }()
	if hit, ok := s.searches[query]; ok && time.Now().Before(hit.expires) {
		return cloneTracks(hit.tracks), nil
	}
	if err = s.ensureUser(ctx, client, session); err != nil {
		return nil, err
	}
	inner, err := s.alAudio(ctx, client, session, url.Values{"act": {"section"}, "al": {"1"}, "claim": {"0"}, "is_layer": {"0"}, "owner_id": {strconv.FormatInt(s.uid, 10)}, "section": {"search"}, "q": {query}})
	if err != nil {
		return nil, err
	}
	if len(inner) < 2 {
		return nil, &Error{Stage: "section", Kind: "json"}
	}
	audios, err := searchAudio(inner[1])
	if err != nil {
		return nil, &Error{Stage: "section", Kind: "json"}
	}
	tracks := []domain.Track{}
	seen := map[string]bool{}
	for _, a := range audios {
		id := a.trackID()
		if id == "" || seen[id] || !domain.IsSongDuration(a.Duration*1000) || a.isRestricted() {
			continue
		}
		seen[id] = true
		tracks = append(tracks, a.track())
		if len(s.candidates) >= 4096 {
			clear(s.candidates)
		}
		// Decode direct streams only for a track that is actually played.
		s.candidates[id] = a
	}
	if !s.unchanged(gen) {
		return nil, &Error{Stage: "section", Kind: "session"}
	}
	if len(s.searches) >= 128 {
		clear(s.searches)
	}
	ttl := 10 * time.Minute
	if len(tracks) == 0 {
		ttl = 30 * time.Second
	}
	s.searches[query] = cachedSearch{tracks: cloneTracks(tracks), expires: time.Now().Add(ttl)}
	return tracks, nil
}
func cloneTracks(tracks []domain.Track) []domain.Track {
	copyTracks := append([]domain.Track{}, tracks...)
	for i := range copyTracks {
		copyTracks[i].Artists = append([]string{}, tracks[i].Artists...)
	}
	return copyTracks
}

var siteTrackID = regexp.MustCompile(`^site_(-?\d+)_(\d+)_([A-Za-z0-9-]+)_([A-Za-z0-9-]+)$`)

func (s *Service) ResolveStream(ctx context.Context, id string) (string, string, error) {
	match := siteTrackID.FindStringSubmatch(id)
	if match == nil {
		return "", "", &Error{Stage: "reload_audio", Kind: "legacy"}
	}
	client, session, gen, err := s.acquire(ctx, "reload_audio")
	if err != nil {
		return "", "", err
	}
	defer func() { <-s.gate }()
	if hit, ok := s.streams[id]; ok && time.Now().Before(hit.expires) {
		return hit.target, hit.mime, nil
	}
	if a, ok := s.candidates[id]; ok {
		if target, mime := a.stream(s.uid); target != "" {
			if !s.unchanged(gen) {
				return "", "", &Error{Stage: "reload_audio", Kind: "session"}
			}
			if len(s.streams) >= 128 {
				clear(s.streams)
			}
			s.streams[id] = cachedStream{target: target, mime: mime, expires: streamExpiry(target)}
			return target, mime, nil
		}
	}
	if err = s.ensureUser(ctx, client, session); err != nil {
		return "", "", err
	}
	inner, err := s.alAudio(ctx, client, session, url.Values{"act": {"reload_audio"}, "al": {"1"}, "ids": {strings.Join(match[1:], "_")}})
	if err != nil {
		return "", "", err
	}
	if len(inner) == 0 {
		return "", "", &Error{Stage: "reload_audio", Kind: "json"}
	}
	audios, err := parseAudioList(inner[0])
	if err != nil {
		return "", "", &Error{Stage: "reload_audio", Kind: "json"}
	}
	owner, _ := strconv.ParseInt(match[1], 10, 64)
	audioID, _ := strconv.ParseInt(match[2], 10, 64)
	for _, a := range audios {
		if a.OwnerID != owner || a.ID != audioID {
			continue
		}
		target, mime := a.stream(s.uid)
		if target == "" {
			break
		}
		if !s.unchanged(gen) {
			return "", "", &Error{Stage: "reload_audio", Kind: "session"}
		}
		if len(s.streams) >= 128 {
			clear(s.streams)
		}
		s.streams[id] = cachedStream{target: target, mime: mime, expires: streamExpiry(target)}
		return target, mime, nil
	}
	return "", "", &Error{Stage: "reload_audio", Kind: "restricted"}
}

func ParseCookies(header, ua string) (Session, error) {
	s := Session{Cookies: strings.TrimSpace(header), UserAgent: ua}
	if !s.Valid() {
		return Session{}, &Error{Stage: "website", Kind: "session"}
	}
	return s, nil
}

// FormatCookies restricts CDP results to VK hosts, preferring the current .ru site.
func FormatCookies(cookies []BrowserCookie) string {
	values := map[string]string{}
	for _, root := range []string{"vk.com", "vk.ru"} {
		for _, c := range cookies {
			host := strings.TrimPrefix(c.Domain, ".")
			if (host == root || strings.HasSuffix(host, "."+root)) && c.Value != "" {
				if h := (&http.Cookie{Name: c.Name, Value: c.Value}).String(); h != "" {
					values[c.Name] = h
				}
			}
		}
	}
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		parts = append(parts, values[key])
	}
	return strings.Join(parts, "; ")
}
