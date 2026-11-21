// Package profile manages one person's migration profile: where its files
// live, which accounts are signed in, and (for M0) which spike checks passed.
//
// One person is one profile. Profiles never share tokens or state. From M1
// the profile directory also holds the SQLite state database.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jeremypoulter/google-sync/internal/config"
	"github.com/jeremypoulter/google-sync/internal/tokenstore"
)

// Account is what we know about a signed-in account.
type Account struct {
	Email      string    `json:"email"`
	Domain     string    `json:"domain,omitempty"` // Workspace "hd" claim; empty for consumer accounts
	SignedInAt time.Time `json:"signed_in_at"`
}

// Check records the outcome of one M0 spike check.
type Check struct {
	Passed bool      `json:"passed"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

// M0 check names, in the order the plan lists them.
const (
	CheckSignInOld   = "signin-old"
	CheckSignInNew   = "signin-new"
	CheckGmailImport = "gmail-import"
	CheckDriveCopy   = "drive-copy"
	CheckTokenAge    = "token-8-days"
)

// CheckOrder lists the M0 checks with a human label.
var CheckOrder = []struct{ Name, Label string }{
	{CheckSignInOld, "Sign in to the old Workspace account"},
	{CheckSignInNew, "Sign in to the new consumer account"},
	{CheckGmailImport, "Import one email into the new account"},
	{CheckDriveCopy, "Copy one Google Doc into the new account"},
	{CheckTokenAge, "Refresh tokens still work after 8+ days"},
}

// Data is the profile.json content.
type Data struct {
	Name     string                   `json:"name"`
	Accounts map[config.Role]*Account `json:"accounts"`
	Checks   map[string]*Check        `json:"checks"`
	// PendingUnshares are temporary Drive shares that could not be removed;
	// they are retried before the next Drive copy.
	PendingUnshares []PendingUnshare `json:"pending_unshares,omitempty"`
}

// PendingUnshare identifies a temporary share on a source Drive file.
type PendingUnshare struct {
	FileID string `json:"file_id"`
	PermID string `json:"perm_id"`
}

// Profile is an opened profile.
type Profile struct {
	Dir string
	Data
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// DefaultBaseDir is where profiles live unless --data-dir is given:
// %LOCALAPPDATA%\GMigrate on Windows, $XDG_DATA_HOME/gmigrate (or
// ~/.local/share/gmigrate) elsewhere.
func DefaultBaseDir() (string, error) {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "GMigrate"), nil
		}
		d, err := os.UserCacheDir() // %LocalAppData% on Windows
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "GMigrate"), nil
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "gmigrate"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "gmigrate"), nil
}

// Open loads (or creates) the named profile under baseDir.
func Open(baseDir, name string) (*Profile, error) {
	if !nameRE.MatchString(name) {
		return nil, fmt.Errorf("invalid profile name %q: use letters, digits, - and _", name)
	}
	if baseDir == "" {
		var err error
		if baseDir, err = DefaultBaseDir(); err != nil {
			return nil, err
		}
	}
	p := &Profile{Dir: filepath.Join(baseDir, "profiles", name)}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p.dataPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		p.Name = name
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &p.Data); err != nil {
			return nil, fmt.Errorf("%s: %w", p.dataPath(), err)
		}
	}
	if p.Accounts == nil {
		p.Accounts = map[config.Role]*Account{}
	}
	if p.Checks == nil {
		p.Checks = map[string]*Check{}
	}
	return p, nil
}

func (p *Profile) dataPath() string { return filepath.Join(p.Dir, "profile.json") }

// TokenPath is the encrypted token file for a role.
func (p *Profile) TokenPath(r config.Role) string {
	return filepath.Join(p.Dir, "tokens", string(r)+".tok")
}

// LoadToken reads the stored token record for a role.
func (p *Profile) LoadToken(r config.Role) (*tokenstore.Record, error) {
	return tokenstore.Load(p.TokenPath(r))
}

// SaveToken writes the token record for a role.
func (p *Profile) SaveToken(r config.Role, rec *tokenstore.Record) error {
	return tokenstore.Save(p.TokenPath(r), rec)
}

// SetCheck records a check result and saves the profile.
func (p *Profile) SetCheck(name string, passed bool, detail string) error {
	p.Checks[name] = &Check{Passed: passed, At: time.Now().UTC(), Detail: detail}
	return p.Save()
}

// ResetChecksForSignIn clears check results that a new sign-in for role makes
// stale: the token-age check always (the new token starts at age zero), and
// the copy checks too when the account itself changed.
func (p *Profile) ResetChecksForSignIn(r config.Role, newEmail string) {
	delete(p.Checks, CheckTokenAge)
	if prev := p.Accounts[r]; prev == nil || !strings.EqualFold(prev.Email, newEmail) {
		delete(p.Checks, CheckGmailImport)
		delete(p.Checks, CheckDriveCopy)
	}
}

// Save writes profile.json.
func (p *Profile) Save() error {
	b, err := json.MarshalIndent(&p.Data, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.dataPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.dataPath())
}
