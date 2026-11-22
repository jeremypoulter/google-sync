package tokenstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "old.tok")
	want := &Record{
		Token:      &oauth2.Token{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer", Expiry: time.Unix(1700000000, 0).UTC()},
		ObtainedAt: time.Unix(1690000000, 0).UTC(),
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token.RefreshToken != "r" || got.Token.AccessToken != "a" || !got.ObtainedAt.Equal(want.ObtainedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for non-token file")
	}
}

func TestAge(t *testing.T) {
	now := time.Now()
	r := &Record{ObtainedAt: now.Add(-9 * 24 * time.Hour)}
	if r.Age(now) < 8*24*time.Hour {
		t.Fatal("age too small")
	}
}
