package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/jeremypoulter/google-sync/internal/config"
)

func fakeIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}

func TestIdentityAndRoleChecks(t *testing.T) {
	ws := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": fakeIDToken(t, map[string]any{"email": "a@family.example", "hd": "family.example"})})
	gm := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": fakeIDToken(t, map[string]any{"email": "a@gmail.com"})})

	wsID, err := IdentityFromToken(ws)
	if err != nil {
		t.Fatal(err)
	}
	gmID, err := IdentityFromToken(gm)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		role   config.Role
		id     *Identity
		domain string
		ok     bool
	}{
		{config.RoleOld, wsID, "", true},
		{config.RoleOld, wsID, "FAMILY.example", true},
		{config.RoleOld, wsID, "other.example", false},
		{config.RoleOld, gmID, "", false},
		{config.RoleNew, gmID, "", true},
		{config.RoleNew, wsID, "", false},
	}
	for _, c := range cases {
		err := CheckRole(c.role, c.id, c.domain)
		if (err == nil) != c.ok {
			t.Errorf("CheckRole(%s, %s, %q) err=%v, want ok=%v", c.role, c.id.Email, c.domain, err, c.ok)
		}
	}

	if _, err := IdentityFromToken(&oauth2.Token{}); err == nil {
		t.Error("expected error with no id_token")
	}
}

// TestSignInLoopback drives the whole loopback flow against a fake token
// endpoint, with a "browser" that follows the consent URL's redirect.
func TestSignInLoopback(t *testing.T) {
	var gotVerifier string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "the-code" {
			http.Error(w, "bad code", 400)
			return
		}
		gotVerifier = r.Form.Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	cfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: tokenSrv.URL},
		Scopes:   []string{"openid"},
	}
	browser := func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		if q.Get("code_challenge") == "" || q.Get("access_type") != "offline" || !strings.Contains(q.Get("prompt"), "consent") {
			return fmt.Errorf("auth URL missing PKCE/offline/consent: %s", authURL)
		}
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=the-code")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
		return nil
	}

	tok, err := SignIn(context.Background(), cfg, SignInOptions{OpenBrowser: browser, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if tok.RefreshToken != "rt" {
		t.Fatalf("refresh token = %q", tok.RefreshToken)
	}
	if gotVerifier == "" {
		t.Fatal("code_verifier not sent on exchange")
	}
}

func TestSignInStateMismatch(t *testing.T) {
	cfg := &oauth2.Config{ClientID: "cid", Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: "http://127.0.0.1:1/"}}
	browser := func(authURL string) error {
		u, _ := url.Parse(authURL)
		go http.Get(u.Query().Get("redirect_uri") + "?state=wrong&code=x") //nolint:errcheck
		return nil
	}
	_, err := SignIn(context.Background(), cfg, SignInOptions{OpenBrowser: browser, Timeout: 10 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("err = %v, want state mismatch", err)
	}
}
