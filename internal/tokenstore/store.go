// Package tokenstore keeps OAuth tokens on disk, encrypted for the current
// user where the platform allows it (Windows DPAPI). Elsewhere the file is
// written with 0600 permissions; a Secret Service backend can come later.
package tokenstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// Record is what is stored for one account.
type Record struct {
	Token *oauth2.Token `json:"token"`
	// ObtainedAt is when the refresh token was first issued by a sign-in. It
	// is kept across refreshes so the M0 check can tell how old it is.
	ObtainedAt time.Time `json:"obtained_at"`
	// LastRefresh is the last time the refresh token was successfully used.
	LastRefresh time.Time `json:"last_refresh,omitempty"`
}

// Age is how long ago the refresh token was issued.
func (r *Record) Age(now time.Time) time.Duration { return now.Sub(r.ObtainedAt) }

const (
	magic        = "GMT1"
	flagPlain    = byte(0)
	flagDPAPI    = byte(1)
	headerLength = len(magic) + 1
)

// ErrNotFound means there is no saved token for that account.
var ErrNotFound = errors.New("not signed in")

// Save writes rec to path, encrypting it if the platform supports it.
func Save(path string, rec *Record) error {
	plain, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	flag := flagPlain
	payload := plain
	if canProtect {
		payload, err = protect(plain)
		if err != nil {
			return fmt.Errorf("encrypting token: %w", err)
		}
		flag = flagDPAPI
	}
	var buf bytes.Buffer
	buf.WriteString(magic)
	buf.WriteByte(flag)
	buf.Write(payload)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a record written by Save.
func Load(path string) (*Record, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(b) < headerLength || string(b[:len(magic)]) != magic {
		return nil, fmt.Errorf("%s: not a token file", path)
	}
	payload := b[headerLength:]
	switch b[len(magic)] {
	case flagPlain:
	case flagDPAPI:
		if !canProtect {
			return nil, fmt.Errorf("%s was encrypted on Windows and cannot be read here", path)
		}
		payload, err = unprotect(payload)
		if err != nil {
			return nil, fmt.Errorf("decrypting token (was it saved by another Windows user?): %w", err)
		}
	default:
		return nil, fmt.Errorf("%s: unknown token encoding", path)
	}
	var rec Record
	if err := json.Unmarshal(payload, &rec); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &rec, nil
}
