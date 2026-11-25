package vkweb

import (
	"encoding/base64"
	"encoding/json"
)

type BrowserCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

func EncodeSession(s Session) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sealed, err := protect(raw)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
func DecodeSession(encoded string) (Session, error) {
	var s Session
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return s, &Error{Kind: "session"}
	}
	plain, err := unprotect(raw)
	if err != nil {
		return s, &Error{Kind: "session"}
	}
	if json.Unmarshal(plain, &s) != nil || !s.Valid() {
		return Session{}, &Error{Kind: "session"}
	}
	return s, nil
}
