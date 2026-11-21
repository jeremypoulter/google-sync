package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/jeremypoulter/google-sync/internal/auth"
	"github.com/jeremypoulter/google-sync/internal/config"
	"github.com/jeremypoulter/google-sync/internal/drivecopy"
	"github.com/jeremypoulter/google-sync/internal/gmailcopy"
	"github.com/jeremypoulter/google-sync/internal/profile"
)

// clients returns HTTP clients for both accounts of a profile.
func clients(ctx context.Context, c *common, p *profile.Profile) (src, dst option.ClientOption, err error) {
	for _, r := range []config.Role{config.RoleOld, config.RoleNew} {
		cfg, err := config.OAuthConfig(r, c.clientFile)
		if err != nil {
			return nil, nil, err
		}
		hc, err := auth.Client(ctx, p, r, cfg)
		if err != nil {
			return nil, nil, err
		}
		if r == config.RoleOld {
			src = option.WithHTTPClient(hc)
		} else {
			dst = option.WithHTTPClient(hc)
		}
	}
	return src, dst, nil
}

func cmdSpikeGmail(ctx context.Context, args []string, out io.Writer) error {
	var c common
	fs := newFlags("spike gmail", &c)
	msgID := fs.String("message", "", "source Gmail message ID (default: the most recent message)")
	prefix := fs.String("label-prefix", "", `prefix for user labels in the new account, e.g. "Old/"`)
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	p, err := c.open()
	if err != nil {
		return err
	}
	srcOpt, dstOpt, err := clients(ctx, &c, p)
	if err != nil {
		return err
	}
	src, err := gmail.NewService(ctx, srcOpt)
	if err != nil {
		return err
	}
	dst, err := gmail.NewService(ctx, dstOpt)
	if err != nil {
		return err
	}

	id := *msgID
	if id == "" {
		list, err := src.Users.Messages.List("me").Q("-in:chats -in:drafts").MaxResults(1).Context(ctx).Do()
		if err != nil {
			return fail(p, profile.CheckGmailImport, fmt.Errorf("finding a message to copy: %w", err))
		}
		if len(list.Messages) == 0 {
			return fail(p, profile.CheckGmailImport, errors.New("the old mailbox has no messages to copy"))
		}
		id = list.Messages[0].Id
	}

	cp := &gmailcopy.Copier{Src: src, Dst: dst, Opt: gmailcopy.Options{LabelPrefix: *prefix}}
	start := time.Now()
	res, err := cp.Copy(ctx, id)
	if err != nil {
		return fail(p, profile.CheckGmailImport, err)
	}
	verb := "Imported"
	if res.Skipped {
		verb = "Already in the new account (duplicate check worked), not imported again:"
	}
	fmt.Fprintf(out, "%s message %s\n  Message-ID: %s\n  new account ID: %s\n  labels: %v\n  took %s\n",
		verb, res.SourceID, res.MessageID, res.DestID, res.Labels, time.Since(start).Round(time.Millisecond))
	return p.SetCheck(profile.CheckGmailImport, true, fmt.Sprintf("%s -> %s", res.MessageID, res.DestID))
}

func cmdSpikeDrive(ctx context.Context, args []string, out io.Writer) error {
	var c common
	fs := newFlags("spike drive", &c)
	fileID := fs.String("file", "", "source Drive file ID (default: your most recently edited Google Doc)")
	folder := fs.String("folder", "", "destination folder ID in the new account (default: My Drive)")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	p, err := c.open()
	if err != nil {
		return err
	}
	dstAcct := p.Accounts[config.RoleNew]
	if dstAcct == nil {
		return errors.New("new account: not signed in (run: gmigrate signin new)")
	}
	srcOpt, dstOpt, err := clients(ctx, &c, p)
	if err != nil {
		return err
	}
	src, err := drive.NewService(ctx, srcOpt)
	if err != nil {
		return err
	}
	dst, err := drive.NewService(ctx, dstOpt)
	if err != nil {
		return err
	}

	cp := &drivecopy.Copier{Src: src, Dst: dst, DstEmail: dstAcct.Email, DstFolder: *folder}

	// Remove any temporary shares an earlier run could not clean up.
	if len(p.PendingUnshares) > 0 {
		var still []profile.PendingUnshare
		for _, pu := range p.PendingUnshares {
			if err := cp.Unshare(ctx, pu.FileID, pu.PermID); err != nil {
				fmt.Fprintf(out, "Still cannot remove the temporary share on %s: %v\n", pu.FileID, err)
				still = append(still, pu)
				continue
			}
			fmt.Fprintf(out, "Removed a leftover temporary share on %s\n", pu.FileID)
		}
		p.PendingUnshares = still
		if err := p.Save(); err != nil {
			return err
		}
		if len(still) > 0 {
			return fail(p, profile.CheckDriveCopy, errors.New("temporary shares from an earlier run are still in place; try again later"))
		}
	}

	id := *fileID
	if id == "" {
		list, err := src.Files.List().
			Q("mimeType = 'application/vnd.google-apps.document' and 'me' in owners and trashed = false").
			OrderBy("modifiedTime desc").PageSize(1).Fields("files(id,name)").Context(ctx).Do()
		if err != nil {
			return fail(p, profile.CheckDriveCopy, fmt.Errorf("finding a Doc to copy: %w", err))
		}
		if len(list.Files) == 0 {
			return fail(p, profile.CheckDriveCopy, errors.New("no Google Docs owned by the old account; pass --file"))
		}
		id = list.Files[0].Id
	}

	start := time.Now()
	res, err := cp.Copy(ctx, id)
	var cleanup *drivecopy.CleanupError
	if errors.As(err, &cleanup) {
		p.PendingUnshares = append(p.PendingUnshares, profile.PendingUnshare{FileID: cleanup.FileID, PermID: cleanup.PermID})
		if res != nil {
			fmt.Fprintf(out, "Copied %q to %s, but the temporary share could not be removed; it will be retried on the next run.\n", res.Name, res.DestID)
		}
		return fail(p, profile.CheckDriveCopy, err)
	}
	if err != nil {
		return fail(p, profile.CheckDriveCopy, err)
	}
	switch {
	case res.Skipped:
		fmt.Fprintf(out, "Already copied (duplicate check worked): %q\n", res.Name)
	case res.Fallback:
		fmt.Fprintf(out, "Copied %q by download and upload (server-side copy was refused)\n", res.Name)
	default:
		fmt.Fprintf(out, "Copied %q server side\n", res.Name)
	}
	fmt.Fprintf(out, "  type: %s\n  new account file: %s\n  link: %s\n  took %s\n",
		res.MimeType, res.DestID, res.DestLink, time.Since(start).Round(time.Millisecond))
	return p.SetCheck(profile.CheckDriveCopy, true, fmt.Sprintf("%q -> %s", res.Name, res.DestID))
}

func cmdTokenCheck(ctx context.Context, args []string, out io.Writer) error {
	var c common
	fs := newFlags("token-check", &c)
	minAge := fs.Duration("min-age", 8*24*time.Hour, "age both tokens must reach for the check to pass")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	p, err := c.open()
	if err != nil {
		return err
	}
	now := time.Now()
	var failures []string
	youngest := time.Duration(1<<63 - 1)
	for _, r := range []config.Role{config.RoleOld, config.RoleNew} {
		cfg, err := config.OAuthConfig(r, c.clientFile)
		if err != nil {
			return err
		}
		rec, err := auth.ForceRefresh(ctx, p, r, cfg)
		if rec == nil {
			return fmt.Errorf("%s account: %w (run: gmigrate signin %s)", r, err, r)
		}
		age := rec.Age(now)
		if age < youngest {
			youngest = age
		}
		if err != nil {
			msg := fmt.Sprintf("%s account: refresh FAILED after %s: %v", r, formatAge(age), describeRefreshError(err))
			fmt.Fprintln(out, msg)
			failures = append(failures, msg)
			continue
		}
		fmt.Fprintf(out, "%s account: refresh OK, token issued %s ago\n", r, formatAge(age))
	}
	if len(failures) > 0 {
		_ = p.SetCheck(profile.CheckTokenAge, false, failures[0])
		return errors.New("token refresh failed: the auth design needs to change (see docs/m0-spike.md)")
	}
	if youngest < *minAge {
		due := now.Add(*minAge - youngest).Format("Mon 2 Jan 15:04")
		fmt.Fprintf(out, "\nBoth refresh tokens work. Run token-check again after %s to complete the check.\n", due)
		return nil
	}
	fmt.Fprintf(out, "\nBoth refresh tokens still work after %s. Check passed.\n", formatAge(youngest))
	return p.SetCheck(profile.CheckTokenAge, true, fmt.Sprintf("refreshed OK at %s old", formatAge(youngest)))
}

func describeRefreshError(err error) string {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.ErrorCode == "invalid_grant" {
		return "Google rejected the refresh token (invalid_grant: expired or revoked)"
	}
	return err.Error()
}

func fail(p *profile.Profile, check string, err error) error {
	_ = p.SetCheck(check, false, err.Error())
	return err
}
