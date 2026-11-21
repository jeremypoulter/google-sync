package drivecopy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func newService(t *testing.T, h http.Handler) *drive.Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := drive.NewService(context.Background(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type fakeSrc struct {
	shared, unshared bool
	shareEmail       string
	notify           string
	alreadyShared    bool // the new account already has access
	unshareFails     bool
}

func (s *fakeSrc) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files/doc1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"id": "doc1", "name": "Plan", "mimeType": "application/vnd.google-apps.document", "modifiedTime": "2026-01-01T00:00:00Z"})
	})
	mux.HandleFunc("GET /files/doc1/permissions", func(w http.ResponseWriter, r *http.Request) {
		perms := []map[string]string{{"id": "owner", "type": "user", "emailAddress": "me@family.example"}}
		if s.alreadyShared {
			perms = append(perms, map[string]string{"id": "permX", "type": "user", "emailAddress": "Me@Gmail.com"})
		}
		writeJSON(w, map[string]any{"permissions": perms})
	})
	mux.HandleFunc("POST /files/doc1/permissions", func(w http.ResponseWriter, r *http.Request) {
		var p drive.Permission
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.shared, s.shareEmail, s.notify = true, p.EmailAddress, r.URL.Query().Get("sendNotificationEmail")
		writeJSON(w, map[string]string{"id": "perm1"})
	})
	mux.HandleFunc("DELETE /files/doc1/permissions/perm1", func(w http.ResponseWriter, r *http.Request) {
		if s.unshareFails {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":{"code":500,"message":"backend error"}}`))
			return
		}
		s.unshared = true
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

type fakeDst struct {
	existing    bool
	copyMeta    drive.File
	copyAttempt int
	notFoundFor int // return 404 for this many copy attempts
}

func (d *fakeDst) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("q"), "value='doc1'") {
			http.Error(w, "bad query "+r.URL.Query().Get("q"), 400)
			return
		}
		if d.existing {
			writeJSON(w, map[string]any{"files": []map[string]string{{"id": "copy-old"}}})
			return
		}
		writeJSON(w, map[string]any{"files": []any{}})
	})
	mux.HandleFunc("POST /files/doc1/copy", func(w http.ResponseWriter, r *http.Request) {
		d.copyAttempt++
		if d.copyAttempt <= d.notFoundFor {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"File not found"}}`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&d.copyMeta)
		writeJSON(w, map[string]string{"id": "copy-new", "webViewLink": "https://docs.example/copy-new"})
	})
	return mux
}

func TestCopySharesCopiesAndUnshares(t *testing.T) {
	src, dst := &fakeSrc{}, &fakeDst{notFoundFor: 2}
	c := &Copier{
		Src: newService(t, src.handler()), Dst: newService(t, dst.handler()),
		DstEmail: "me@gmail.com", DstFolder: "folderX", Sleep: func(time.Duration) {},
	}
	res, err := c.Copy(context.Background(), "doc1")
	if err != nil {
		t.Fatal(err)
	}
	if res.DestID != "copy-new" || res.Skipped {
		t.Fatalf("unexpected result %+v", res)
	}
	if !src.shared || src.shareEmail != "me@gmail.com" || src.notify != "false" {
		t.Errorf("share wrong: %+v", src)
	}
	if !src.unshared {
		t.Error("temporary share was not removed")
	}
	if dst.copyAttempt != 3 {
		t.Errorf("copy attempts = %d, want 3 (two 404 retries)", dst.copyAttempt)
	}
	if dst.copyMeta.AppProperties[PropSrcID] != "doc1" || len(dst.copyMeta.Parents) != 1 || dst.copyMeta.Parents[0] != "folderX" {
		t.Errorf("copy metadata = %+v", dst.copyMeta)
	}
}

func TestCopySkipsExisting(t *testing.T) {
	src, dst := &fakeSrc{}, &fakeDst{existing: true}
	c := &Copier{Src: newService(t, src.handler()), Dst: newService(t, dst.handler()), DstEmail: "me@gmail.com"}
	res, err := c.Copy(context.Background(), "doc1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped || res.DestID != "copy-old" {
		t.Fatalf("expected skip, got %+v", res)
	}
	if src.shared {
		t.Error("shared even though a copy exists")
	}
}

func TestCopyDefaultsToRootAndKeepsExistingShare(t *testing.T) {
	src, dst := &fakeSrc{alreadyShared: true}, &fakeDst{}
	c := &Copier{Src: newService(t, src.handler()), Dst: newService(t, dst.handler()), DstEmail: "me@gmail.com"}
	if _, err := c.Copy(context.Background(), "doc1"); err != nil {
		t.Fatal(err)
	}
	if src.shared || src.unshared {
		t.Errorf("existing share must be reused and kept: %+v", src)
	}
	if len(dst.copyMeta.Parents) != 1 || dst.copyMeta.Parents[0] != "root" {
		t.Errorf("parents = %v, want [root]", dst.copyMeta.Parents)
	}
}

func TestCopyReportsFailedCleanup(t *testing.T) {
	src, dst := &fakeSrc{unshareFails: true}, &fakeDst{}
	c := &Copier{Src: newService(t, src.handler()), Dst: newService(t, dst.handler()), DstEmail: "me@gmail.com"}
	res, err := c.Copy(context.Background(), "doc1")
	var ce *CleanupError
	if !errors.As(err, &ce) || ce.FileID != "doc1" || ce.PermID != "perm1" {
		t.Fatalf("err = %v, want CleanupError for doc1/perm1", err)
	}
	if res == nil || res.DestID != "copy-new" {
		t.Fatalf("result should still report the copy: %+v", res)
	}
	src.unshareFails = false
	if err := c.Unshare(context.Background(), ce.FileID, ce.PermID); err != nil || !src.unshared {
		t.Fatalf("retrying cleanup: err=%v unshared=%v", err, src.unshared)
	}
}

func TestEscapeQuery(t *testing.T) {
	if got := escapeQuery(`a'b\c`); got != `a\'b\\c` {
		t.Fatalf("got %s", got)
	}
}
