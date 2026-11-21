package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpAndUsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "signin") {
		t.Fatalf("help: code %d, out %q", code, out.String())
	}
	errOut.Reset()
	if code := run(context.Background(), []string{"bogus"}, &out, &errOut); code != 2 {
		t.Fatalf("unknown command: code %d", code)
	}
	if code := run(context.Background(), []string{"signin", "sideways", "--data-dir", t.TempDir()}, &out, &errOut); code != 2 {
		t.Fatalf("bad role: code %d, stderr %q", code, errOut.String())
	}
}

func TestStatusEmptyProfile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"status", "--data-dir", t.TempDir(), "--profile", "kid1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	s := out.String()
	if !strings.Contains(s, `Profile "kid1"`) || !strings.Contains(s, "0 of 5 checks passed") {
		t.Fatalf("unexpected status output:\n%s", s)
	}
}

func TestSignInWithoutClient(t *testing.T) {
	t.Setenv("GMIGRATE_CLIENT_ID", "")
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"signin", "old", "--data-dir", t.TempDir()}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "no OAuth client") {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
}
