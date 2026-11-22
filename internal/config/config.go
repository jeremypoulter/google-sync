// Package config holds the OAuth client credentials and the scopes each side
// of the migration asks for.
//
// The client ID and secret are normally built into the executable with
//
//	go build -ldflags "-X github.com/jeremypoulter/google-sync/internal/config.ClientID=... \
//	                   -X github.com/jeremypoulter/google-sync/internal/config.ClientSecret=..."
//
// so family members get a ready-to-run file. For development they can also
// come from the GMIGRATE_CLIENT_ID / GMIGRATE_CLIENT_SECRET environment
// variables or from the client JSON file downloaded from Google Cloud.
//
// Google does not treat desktop-app client secrets as confidential (PKCE and
// the loopback redirect protect the flow), so embedding them is acceptable for
// a private build. Do not publish a build with the secret in it.
package config

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Set at build time with -ldflags -X.
var (
	ClientID     = ""
	ClientSecret = ""
	Version      = "dev"
)

// Role says which side of the migration an account is on.
type Role string

const (
	// RoleOld is the Workspace account being migrated from (read only).
	RoleOld Role = "old"
	// RoleNew is the consumer Google account being migrated to.
	RoleNew Role = "new"
)

// ParseRole accepts "old"/"source" and "new"/"dest"/"destination".
func ParseRole(s string) (Role, error) {
	switch s {
	case "old", "source", "src":
		return RoleOld, nil
	case "new", "dest", "destination", "dst":
		return RoleNew, nil
	}
	return "", fmt.Errorf("unknown account %q: use \"old\" or \"new\"", s)
}

// identityScopes let the tool learn which account was picked, and whether it
// belongs to a Workspace domain (the "hd" claim).
var identityScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
}

// OldScopes are requested from the Workspace (source) account. Everything is
// read only except Drive, which needs full access so the tool can share each
// file to the new account before copying it server side.
var OldScopes = append([]string{
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/drive",
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/contacts.readonly",
	"https://www.googleapis.com/auth/contacts.other.readonly",
	"https://www.googleapis.com/auth/tasks.readonly",
}, identityScopes...)

// NewScopes are requested from the consumer (destination) account.
//
// gmail.readonly is needed in addition to gmail.insert: duplicate detection
// searches the destination for the source Message-ID (rfc822msgid:), and
// messages.list with a query is not allowed under insert/labels alone.
var NewScopes = append([]string{
	"https://www.googleapis.com/auth/gmail.insert",
	"https://www.googleapis.com/auth/gmail.labels",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/drive",
	"https://www.googleapis.com/auth/calendar",
	"https://www.googleapis.com/auth/contacts",
	"https://www.googleapis.com/auth/tasks",
}, identityScopes...)

// Scopes returns the scopes for a role.
func Scopes(r Role) []string {
	if r == RoleOld {
		return OldScopes
	}
	return NewScopes
}

// ErrNoClient means no OAuth client credentials were found anywhere.
var ErrNoClient = errors.New("no OAuth client configured: build with -ldflags to embed one, set GMIGRATE_CLIENT_ID and GMIGRATE_CLIENT_SECRET, or pass --client-file (see docs/admin-setup.md)")

// OAuthConfig builds the oauth2 config for a role. clientFile, if not empty,
// is the client JSON downloaded from Google Cloud and wins over everything
// else; then the environment; then the values built into the binary.
// RedirectURL is left empty: the loopback flow fills it in per sign-in.
func OAuthConfig(r Role, clientFile string) (*oauth2.Config, error) {
	scopes := Scopes(r)
	if clientFile != "" {
		b, err := os.ReadFile(clientFile)
		if err != nil {
			return nil, fmt.Errorf("reading client file: %w", err)
		}
		c, err := google.ConfigFromJSON(b, scopes...)
		if err != nil {
			return nil, fmt.Errorf("parsing client file: %w", err)
		}
		c.RedirectURL = ""
		return c, nil
	}
	id, secret := ClientID, ClientSecret
	if v := os.Getenv("GMIGRATE_CLIENT_ID"); v != "" {
		id = v
		secret = os.Getenv("GMIGRATE_CLIENT_SECRET")
	}
	if id == "" {
		return nil, ErrNoClient
	}
	return &oauth2.Config{
		ClientID:     id,
		ClientSecret: secret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
	}, nil
}
