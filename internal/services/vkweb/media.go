package vkweb

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func (s *Service) Scope() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum := sha256.Sum256([]byte(s.session.Cookies))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Playback URLs only grant access to resources of this track and current session.
func (s *Service) SignMedia(id, target string) string {
	h := hmac.New(sha256.New, []byte(s.Scope()))
	h.Write([]byte(id + "\x00" + target))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Service) VerifyMedia(id, target, signature string) bool {
	return s.HasToken() && hmac.Equal([]byte(signature), []byte(s.SignMedia(id, target)))
}
