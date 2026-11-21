package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/jeremypoulter/google-sync/internal/config"
	"github.com/jeremypoulter/google-sync/internal/profile"
	"github.com/jeremypoulter/google-sync/internal/tokenstore"
)

// TokenSource returns a token source for a signed-in role that writes every
// refreshed token back to the profile, keeping ObtainedAt unchanged.
func TokenSource(ctx context.Context, p *profile.Profile, r config.Role, cfg *oauth2.Config) (oauth2.TokenSource, error) {
	rec, err := p.LoadToken(r)
	if err != nil {
		return nil, fmt.Errorf("%s account: %w (run: gmigrate signin %s)", r, err, r)
	}
	return &savingSource{
		base: cfg.TokenSource(ctx, rec.Token),
		rec:  rec,
		save: func(rec *tokenstore.Record) error { return p.SaveToken(r, rec) },
	}, nil
}

// Client is TokenSource wrapped in an *http.Client.
func Client(ctx context.Context, p *profile.Profile, r config.Role, cfg *oauth2.Config) (*http.Client, error) {
	ts, err := TokenSource(ctx, p, r, cfg)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(ctx, ts), nil
}

type savingSource struct {
	mu   sync.Mutex
	base oauth2.TokenSource
	rec  *tokenstore.Record
	save func(*tokenstore.Record) error
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != s.rec.Token.AccessToken {
		if t.RefreshToken == "" {
			t.RefreshToken = s.rec.Token.RefreshToken
		}
		// Save a candidate first and only adopt it once it is on disk, so a
		// failed save is retried on the next call.
		cand := *s.rec
		cand.Token = t
		cand.LastRefresh = time.Now().UTC()
		if err := s.save(&cand); err != nil {
			return nil, fmt.Errorf("saving refreshed token: %w", err)
		}
		*s.rec = cand
	}
	return t, nil
}

// ForceRefresh uses the stored refresh token right now, even if the access
// token has not expired, and saves the result. It proves the refresh token
// is still accepted by Google.
func ForceRefresh(ctx context.Context, p *profile.Profile, r config.Role, cfg *oauth2.Config) (*tokenstore.Record, error) {
	rec, err := p.LoadToken(r)
	if err != nil {
		return nil, err
	}
	stale := *rec.Token
	stale.AccessToken = ""
	stale.Expiry = time.Now().Add(-time.Hour)
	t, err := cfg.TokenSource(ctx, &stale).Token()
	if err != nil {
		return rec, err
	}
	if t.RefreshToken == "" {
		t.RefreshToken = rec.Token.RefreshToken
	}
	rec.Token = t
	rec.LastRefresh = time.Now().UTC()
	return rec, p.SaveToken(r, rec)
}
