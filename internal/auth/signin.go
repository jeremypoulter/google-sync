// Package auth runs the desktop OAuth sign-in (loopback redirect with PKCE),
// checks the right kind of account was picked, and hands out HTTP clients
// whose refreshed tokens are written back to the profile.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// SignInOptions tune the interactive sign-in.
type SignInOptions struct {
	// LoginHint pre-fills the account chooser with this address.
	LoginHint string
	// OpenBrowser opens the consent URL. If it fails the URL is still printed.
	OpenBrowser func(url string) error
	// Out receives instructions for the person (the URL to open).
	Out io.Writer
	// Timeout bounds how long to wait for the person; default 10 minutes.
	Timeout time.Duration
}

// SignIn runs the loopback flow: it listens on 127.0.0.1, opens the browser
// at Google's consent page, waits for the redirect and exchanges the code.
// cfg.RedirectURL is overwritten on a copy.
func SignIn(ctx context.Context, cfg *oauth2.Config, opt SignInOptions) (*oauth2.Token, error) {
	if opt.Timeout == 0 {
		opt.Timeout = 10 * time.Minute
	}
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting local sign-in listener: %w", err)
	}
	defer func() { _ = ln.Close() }()

	c := *cfg
	c.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	state, err := randomString(24)
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()

	authOpts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		// consent: always return a refresh token; select_account: never
		// silently reuse whichever account the browser is signed in to.
		oauth2.SetAuthURLParam("prompt", "consent select_account"),
	}
	if opt.LoginHint != "" {
		authOpts = append(authOpts, oauth2.SetAuthURLParam("login_hint", opt.LoginHint))
	}
	authURL := c.AuthCodeURL(state, authOpts...)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			var res result
			switch {
			case q.Get("state") != state:
				res.err = errors.New("sign-in response did not match this request (state mismatch); try again")
			case q.Get("error") != "":
				res.err = describeAuthError(q.Get("error"), q.Get("error_description"))
			case q.Get("code") == "":
				res.err = errors.New("sign-in response had no authorization code")
			default:
				res.code = q.Get("code")
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if res.err != nil {
				fmt.Fprintf(w, page, "Sign-in failed", html.EscapeString(res.err.Error()))
			} else {
				fmt.Fprintf(w, page, "Signed in", "You can close this tab and go back to GMigrate.")
			}
			select {
			case done <- res:
			default:
			}
		}),
	}
	go srv.Serve(ln) //nolint:errcheck // returns ErrServerClosed on Shutdown
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer scancel()
		_ = srv.Shutdown(sctx)
	}()

	fmt.Fprintf(opt.Out, "Opening your browser to sign in. If it does not open, visit:\n\n  %s\n\n", authURL)
	if opt.OpenBrowser != nil {
		if err := opt.OpenBrowser(authURL); err != nil {
			fmt.Fprintf(opt.Out, "(could not open the browser automatically: %v)\n", err)
		}
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for sign-in: %w", ctx.Err())
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		tok, err := c.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, fmt.Errorf("exchanging authorization code: %w", err)
		}
		if tok.RefreshToken == "" {
			return nil, errors.New("google did not return a refresh token; remove GMigrate from the account's Third-party connections page and sign in again")
		}
		return tok, nil
	}
}

func describeAuthError(code, desc string) error {
	switch code {
	case "access_denied":
		return errors.New("access was denied. If Google said the app is blocked by your organisation, ask the Workspace admin to mark the GMigrate client ID as Trusted (docs/admin-setup.md); otherwise make sure every permission box was ticked")
	case "admin_policy_enforced":
		return errors.New("your Workspace admin has not allowed this app yet: ask them to mark the GMigrate client ID as Trusted in Admin console > Security > API controls")
	}
	if desc != "" {
		return fmt.Errorf("sign-in error %s: %s", code, desc)
	}
	return fmt.Errorf("sign-in error %s", code)
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const page = `<!doctype html><meta charset="utf-8"><title>GMigrate</title>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">
<h1>%s</h1><p>%s</p></body>`
