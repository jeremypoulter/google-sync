// Command gmigrate copies a person's Google Workspace data into their new
// consumer Google account.
//
// This is the M0 spike build: headless commands that prove the auth design
// and the two core copy operations before the GUI and sync engine are built.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jeremypoulter/google-sync/internal/auth"
	"github.com/jeremypoulter/google-sync/internal/browser"
	"github.com/jeremypoulter/google-sync/internal/config"
	"github.com/jeremypoulter/google-sync/internal/profile"
	"github.com/jeremypoulter/google-sync/internal/tokenstore"
)

const usage = `gmigrate - copy Google Workspace data to a personal Google account

Usage:
  gmigrate <command> [flags]

Commands (M0 spike):
  signin old|new     Sign in to the old Workspace account or the new account
  spike gmail        Copy one email from the old account to the new one
  spike drive        Copy one Google Doc from the old account to the new one
  token-check        Refresh both tokens and record their age (pass at 8+ days)
  status             Show signed-in accounts and which M0 checks have passed
  version            Print the version

Common flags (any command):
  --profile NAME     Profile to use (default "default"); one per person
  --data-dir DIR     Where profiles are stored (default %LOCALAPPDATA%\GMigrate
                     on Windows, ~/.local/share/gmigrate elsewhere)
  --client-file F    OAuth client JSON from Google Cloud, instead of the
                     client built into this exe or GMIGRATE_CLIENT_ID

Run "gmigrate <command> -h" for a command's own flags.
`

type common struct {
	profile    string
	dataDir    string
	clientFile string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.profile, "profile", "default", "profile name")
	fs.StringVar(&c.dataDir, "data-dir", "", "profile storage directory")
	fs.StringVar(&c.clientFile, "client-file", "", "OAuth client JSON file")
}

func (c *common) open() (*profile.Profile, error) {
	return profile.Open(c.dataDir, c.profile)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	var err error
	switch args[0] {
	case "signin":
		err = cmdSignIn(ctx, args[1:], stdout)
	case "spike":
		if len(args) < 2 {
			err = usageError("spike needs a target: gmail or drive")
			break
		}
		switch args[1] {
		case "gmail":
			err = cmdSpikeGmail(ctx, args[2:], stdout)
		case "drive":
			err = cmdSpikeDrive(ctx, args[2:], stdout)
		default:
			err = usageError("unknown spike target %q: use gmail or drive", args[1])
		}
	case "token-check":
		err = cmdTokenCheck(ctx, args[1:], stdout)
	case "status":
		err = cmdStatus(args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, "gmigrate", config.Version)
	default:
		err = usageError("unknown command %q", args[0])
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	fmt.Fprintln(stderr, "error:", err)
	var ue usageErr
	if errors.As(err, &ue) {
		fmt.Fprintln(stderr, `run "gmigrate help" for usage`)
		return 2
	}
	return 1
}

type usageErr struct{ msg string }

func (u usageErr) Error() string { return u.msg }

func usageError(format string, a ...any) error { return usageErr{fmt.Sprintf(format, a...)} }

func newFlags(name string, c *common) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	c.register(fs)
	return fs
}

// parseInterspersed lets flags appear before or after positional arguments
// ("signin old --profile x" and "signin --profile x old" both work).
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func cmdSignIn(ctx context.Context, args []string, out io.Writer) error {
	var c common
	fs := newFlags("signin", &c)
	domain := fs.String("domain", "", "old account must be on this Workspace domain")
	hint := fs.String("login-hint", "", "email address to pre-select in the browser")
	noBrowser := fs.Bool("no-browser", false, "print the sign-in URL instead of opening a browser")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("signin needs exactly one account: old or new")
	}
	role, err := config.ParseRole(pos[0])
	if err != nil {
		return usageError("%v", err)
	}
	p, err := c.open()
	if err != nil {
		return err
	}
	cfg, err := config.OAuthConfig(role, c.clientFile)
	if err != nil {
		return err
	}

	which := map[config.Role]string{config.RoleOld: "OLD Workspace", config.RoleNew: "NEW personal"}[role]
	fmt.Fprintf(out, "Signing in to your %s Google account for profile %q.\n", which, p.Name)
	fmt.Fprintln(out, `Google will say "Google hasn't verified this app". That is expected for this`)
	fmt.Fprintln(out, `family-only tool: click "Advanced", then "Go to GMigrate (unsafe)", then tick every box.`)
	fmt.Fprintln(out)

	opt := auth.SignInOptions{LoginHint: *hint, Out: out}
	if !*noBrowser {
		opt.OpenBrowser = browser.Open
	}
	tok, err := auth.SignIn(ctx, cfg, opt)
	if err != nil {
		check := profile.CheckSignInOld
		if role == config.RoleNew {
			check = profile.CheckSignInNew
		}
		_ = p.SetCheck(check, false, err.Error())
		return err
	}
	id, err := auth.IdentityFromToken(tok)
	if err != nil {
		return err
	}
	if err := auth.CheckRole(role, id, *domain); err != nil {
		return fmt.Errorf("wrong account: %w. Nothing was saved; run signin again and pick the other account", err)
	}
	// Refuse to pair the same address on both sides.
	other := config.RoleNew
	if role == config.RoleNew {
		other = config.RoleOld
	}
	if a := p.Accounts[other]; a != nil && strings.EqualFold(a.Email, id.Email) {
		return fmt.Errorf("%s is already this profile's %s account", id.Email, other)
	}

	p.ResetChecksForSignIn(role, id.Email)
	now := time.Now().UTC()
	if err := p.SaveToken(role, &tokenstore.Record{Token: tok, ObtainedAt: now}); err != nil {
		return err
	}
	p.Accounts[role] = &profile.Account{Email: id.Email, Domain: id.HostedDomain, SignedInAt: now}
	check := profile.CheckSignInOld
	if role == config.RoleNew {
		check = profile.CheckSignInNew
	}
	if err := p.SetCheck(check, true, id.Email); err != nil {
		return err
	}
	fmt.Fprintf(out, "Signed in as %s", id.Email)
	if id.HostedDomain != "" {
		fmt.Fprintf(out, " (Workspace domain %s)", id.HostedDomain)
	}
	fmt.Fprintln(out, ".")
	return nil
}

func cmdStatus(args []string, out io.Writer) error {
	var c common
	fs := newFlags("status", &c)
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	p, err := c.open()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Profile %q (%s)\n\n", p.Name, p.Dir)
	now := time.Now()
	for _, r := range []config.Role{config.RoleOld, config.RoleNew} {
		a := p.Accounts[r]
		if a == nil {
			fmt.Fprintf(out, "  %-4s account: not signed in\n", r)
			continue
		}
		age := ""
		if rec, err := p.LoadToken(r); err == nil {
			age = fmt.Sprintf(", token issued %s ago", formatAge(rec.Age(now)))
		}
		fmt.Fprintf(out, "  %-4s account: %s%s\n", r, a.Email, age)
	}
	fmt.Fprintln(out, "\nM0 checks:")
	passed := 0
	for _, ch := range profile.CheckOrder {
		res := p.Checks[ch.Name]
		mark, detail := "[ ]", ""
		if res != nil {
			if res.Passed {
				mark = "[x]"
				passed++
			} else {
				mark = "[!]"
			}
			detail = " - " + res.Detail
		}
		fmt.Fprintf(out, "  %s %s%s\n", mark, ch.Label, detail)
	}
	fmt.Fprintf(out, "\n%d of %d checks passed.\n", passed, len(profile.CheckOrder))
	return nil
}

func formatAge(d time.Duration) string {
	if d >= 48*time.Hour {
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	}
	return d.Round(time.Minute).String()
}
