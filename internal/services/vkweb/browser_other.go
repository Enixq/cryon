//go:build !windows

package vkweb

import "context"

func BrowserSupported() bool { return false }
func ConnectBrowser(context.Context) (Session, error) {
	return Session{}, &Error{Stage: "browser", Kind: "browser_missing"}
}
