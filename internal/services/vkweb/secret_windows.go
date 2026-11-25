//go:build windows

package vkweb

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

// DPAPI binds saved website cookies to the current Windows user.
func protect(raw []byte) ([]byte, error)   { return cryptData(raw, true) }
func unprotect(raw []byte) ([]byte, error) { return cryptData(raw, false) }
func cryptData(raw []byte, seal bool) ([]byte, error) {
	if len(raw) == 0 {
		return nil, &Error{Kind: "session"}
	}
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	var err error
	if seal {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, &Error{Kind: "session"}
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
