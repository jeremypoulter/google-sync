//go:build !windows

package tokenstore

// No OS-level encryption yet outside Windows; files are 0600 instead.
const canProtect = false

func protect(b []byte) ([]byte, error)   { return b, nil }
func unprotect(b []byte) ([]byte, error) { return b, nil }
