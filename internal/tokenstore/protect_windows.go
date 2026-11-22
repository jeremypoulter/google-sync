//go:build windows

package tokenstore

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const canProtect = true

// protect encrypts b with DPAPI for the current Windows user.
func protect(b []byte) ([]byte, error) {
	return dpapi(b, true)
}

// unprotect reverses protect. It fails for any other Windows user.
func unprotect(b []byte) ([]byte, error) {
	return dpapi(b, false)
}

func dpapi(b []byte, encrypt bool) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
