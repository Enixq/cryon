//go:build !windows

package vkweb

// Android persists this setting inside the application's private sandbox.
func protect(raw []byte) ([]byte, error)   { return raw, nil }
func unprotect(raw []byte) ([]byte, error) { return raw, nil }
