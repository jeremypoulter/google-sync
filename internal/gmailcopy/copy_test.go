package gmailcopy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const rawMsg = "Message-ID: <abc123@mail.example>\r\nFrom: a@family.example\r\nTo: b@example.com\r\nSubject: hi\r\nDate: Mon, 2 Jan 2023 10:00:00 +0000\r\n\r\nhello\r\n"

func newService(t *testing.T, h http.Handler) *gmail.Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := gmail.NewService(context.Background(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func sourceHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/messages/src1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":       "src1",
			"raw":      base64.URLEncoding.EncodeToString([]byte(rawMsg)),
			"labelIds": []string{"INBOX", "UNREAD", "Label_7", "DRAFT"},
		})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"labels": []map[string]string{
			{"id": "INBOX", "name": "INBOX", "type": "system"},
			{"id": "DRAFT", "name": "DRAFT", "type": "system"},
			{"id": "Label_7", "name": "Receipts", "type": "user"},
		}})
	})
	return mux
}

type fakeDest struct {
	existing      bool
	query         string
	createdLabels []string
	importLabels  []string
	importBody    string
}

func (d *fakeDest) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		d.query = r.URL.Query().Get("q")
		if d.existing {
			writeJSON(w, map[string]any{"messages": []map[string]string{{"id": "dst-old"}}})
			return
		}
		writeJSON(w, map[string]any{})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"labels": []map[string]string{{"id": "INBOX", "name": "INBOX", "type": "system"}}})
	})
	mux.HandleFunc("POST /gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		var l gmail.Label
		_ = json.NewDecoder(r.Body).Decode(&l)
		d.createdLabels = append(d.createdLabels, l.Name)
		writeJSON(w, map[string]string{"id": "Label_new", "name": l.Name})
	})
	mux.HandleFunc("POST /upload/gmail/v1/users/me/messages/import", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("internalDateSource"); got != "dateHeader" {
			t.Errorf("internalDateSource = %q", got)
		}
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		meta, _ := mr.NextPart()
		var m gmail.Message
		_ = json.NewDecoder(meta).Decode(&m)
		d.importLabels = m.LabelIds
		body, _ := mr.NextPart()
		b, _ := io.ReadAll(body)
		d.importBody = string(b)
		writeJSON(w, map[string]string{"id": "dst-new"})
	})
	return mux
}

func TestCopyImportsWithLabels(t *testing.T) {
	d := &fakeDest{}
	c := &Copier{Src: newService(t, sourceHandler()), Dst: newService(t, d.handler(t)), Opt: Options{LabelPrefix: "Old/"}}

	res, err := c.Copy(context.Background(), "src1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped || res.DestID != "dst-new" {
		t.Fatalf("unexpected result %+v", res)
	}
	if d.query != "rfc822msgid:abc123@mail.example" {
		t.Errorf("dedupe query = %q", d.query)
	}
	if strings.Join(d.createdLabels, ",") != "Old/Receipts" {
		t.Errorf("created labels = %v", d.createdLabels)
	}
	if strings.Join(d.importLabels, ",") != "INBOX,UNREAD,Label_new" {
		t.Errorf("import labels = %v (DRAFT must be dropped)", d.importLabels)
	}
	if d.importBody != rawMsg {
		t.Errorf("imported body differs:\n%q", d.importBody)
	}
}

func TestCopySkipsExisting(t *testing.T) {
	d := &fakeDest{existing: true}
	c := &Copier{Src: newService(t, sourceHandler()), Dst: newService(t, d.handler(t))}
	res, err := c.Copy(context.Background(), "src1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped || res.DestID != "dst-old" {
		t.Fatalf("expected skip, got %+v", res)
	}
	if d.importBody != "" {
		t.Fatal("message imported despite existing copy")
	}
}

func TestMessageIDHeader(t *testing.T) {
	if got := MessageIDHeader([]byte(rawMsg)); got != "<abc123@mail.example>" {
		t.Fatalf("got %q", got)
	}
	if got := MessageIDHeader([]byte("not a message")); got != "" {
		t.Fatalf("got %q for garbage", got)
	}
}
