// Package drivecopy copies Drive files from the old account to the new one.
//
// Ownership cannot move from a Workspace account to a consumer account, so
// each file becomes a new copy owned by the new account:
//
//  1. share the source file to the new account as reader (no email),
//  2. files.copy as the new account (server side; native Docs stay native),
//     tagging the copy with appProperties.srcId,
//  3. remove the temporary share.
//
// If copying is refused for a non-Google file, it falls back to downloading
// from the old account and uploading to the new one.
package drivecopy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// AppProperty keys stored on every copy so the mapping can be rebuilt by
// scanning the destination.
const (
	PropSrcID       = "srcId"
	PropSrcModified = "srcModified"
)

// Result reports what happened to one file.
type Result struct {
	SourceID string
	Name     string
	MimeType string
	DestID   string
	DestLink string
	Skipped  bool // a copy tagged with this srcId already exists
	Fallback bool // copied by download + upload instead of files.copy
}

// Copier copies files between two Drive services.
type Copier struct {
	Src, Dst *drive.Service
	// DstEmail is the new account's address, used for the temporary share.
	DstEmail string
	// DstFolder is the destination parent folder ID; empty means My Drive root.
	DstFolder string
	// Sleep is used between retries; tests replace it.
	Sleep func(time.Duration)
}

const srcFields = "id,name,mimeType,modifiedTime,md5Checksum,size,trashed,capabilities(canCopy,canShare,canDownload)"

// Copy copies one file by its source ID.
func (c *Copier) Copy(ctx context.Context, srcID string) (res *Result, err error) {
	f, err := c.Src.Files.Get(srcID).Fields(srcFields).SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("reading source file %s: %w", srcID, err)
	}
	if f.MimeType == "application/vnd.google-apps.folder" {
		return nil, fmt.Errorf("%q is a folder; the spike copies single files", f.Name)
	}
	res = &Result{SourceID: f.Id, Name: f.Name, MimeType: f.MimeType}

	existing, err := c.FindCopy(ctx, f.Id)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		res.DestID, res.DestLink, res.Skipped = existing.Id, existing.WebViewLink, true
		return res, nil
	}

	meta := &drive.File{
		Name: f.Name,
		AppProperties: map[string]string{
			PropSrcID:       f.Id,
			PropSrcModified: f.ModifiedTime,
		},
	}
	// Always name the parent: without one, files.copy may put the copy in
	// the source's folder (and inherit its sharing) if the new account can
	// see that folder.
	meta.Parents = []string{"root"}
	if c.DstFolder != "" {
		meta.Parents = []string{c.DstFolder}
	}

	permID, created, err := c.share(ctx, f.Id)
	if err != nil {
		return nil, err
	}
	if created {
		defer func() {
			if perr := c.Unshare(ctx, f.Id, permID); perr != nil {
				cleanup := &CleanupError{FileID: f.Id, PermID: permID, Err: perr}
				if err == nil {
					err = cleanup
				} else {
					err = errors.Join(err, cleanup)
				}
			}
		}()
	}

	copied, err := c.copyWithRetry(ctx, f.Id, meta)
	if err != nil && isCopyRefused(err) && !strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
		copied, err = c.downloadUpload(ctx, f, meta)
		res.Fallback = err == nil
	}
	if err != nil {
		return nil, err
	}
	res.DestID, res.DestLink = copied.Id, copied.WebViewLink
	return res, nil
}

// FindCopy returns the destination file tagged with srcId, or nil.
func (c *Copier) FindCopy(ctx context.Context, srcID string) (*drive.File, error) {
	q := fmt.Sprintf("appProperties has { key='%s' and value='%s' } and trashed = false", PropSrcID, escapeQuery(srcID))
	list, err := c.Dst.Files.List().Q(q).Fields("files(id,name,webViewLink)").PageSize(1).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("checking destination for existing copy: %w", err)
	}
	if len(list.Files) > 0 {
		return list.Files[0], nil
	}
	return nil, nil
}

// CleanupError means the file was handled but its temporary share to the new
// account could not be removed. Callers should store FileID and PermID and
// retry with Unshare before trusting later runs.
type CleanupError struct {
	FileID, PermID string
	Err            error
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("temporary share on %s still in place: %v", e.FileID, e.Err)
}

func (e *CleanupError) Unwrap() error { return e.Err }

// share gives the new account read access to the source file. If the account
// already had access, that permission is reused and created is false, so the
// caller must not remove it.
func (c *Copier) share(ctx context.Context, fileID string) (permID string, created bool, err error) {
	// Read every page: a file's permission list can be longer than one page,
	// and missing an existing grant would make us delete it afterwards.
	var found string
	err = c.Src.Permissions.List(fileID).SupportsAllDrives(true).
		Fields("nextPageToken,permissions(id,type,emailAddress)").
		Pages(ctx, func(page *drive.PermissionList) error {
			for _, p := range page.Permissions {
				if p.Type == "user" && strings.EqualFold(p.EmailAddress, c.DstEmail) {
					found = p.Id
				}
			}
			return nil
		})
	if err != nil {
		return "", false, fmt.Errorf("reading sharing of %s: %w", fileID, err)
	}
	if found != "" {
		return found, false, nil
	}
	p, err := c.Src.Permissions.Create(fileID, &drive.Permission{
		Type:         "user",
		Role:         "reader",
		EmailAddress: c.DstEmail,
	}).SendNotificationEmail(false).SupportsAllDrives(true).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", false, fmt.Errorf("sharing file to %s (is sharing outside the domain allowed in the Admin console?): %w", c.DstEmail, err)
	}
	return p.Id, true, nil
}

// Unshare removes a temporary share created by Copy. A share that is already
// gone counts as removed.
func (c *Copier) Unshare(ctx context.Context, fileID, permID string) error {
	// Use a fresh context so a cancelled run still removes the share.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err := c.Src.Permissions.Delete(fileID, permID).SupportsAllDrives(true).Context(cctx).Do()
	if err != nil && !isStatus(err, http.StatusNotFound) {
		return fmt.Errorf("removing temporary share on %s: %w", fileID, err)
	}
	return nil
}

// copyWithRetry calls files.copy as the destination, retrying 404s for a
// short while because a brand-new share can take a moment to propagate.
func (c *Copier) copyWithRetry(ctx context.Context, srcID string, meta *drive.File) (*drive.File, error) {
	sleep := c.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	delay := time.Second
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		f, err := c.Dst.Files.Copy(srcID, meta).Fields("id,name,webViewLink").Context(ctx).Do()
		if err == nil {
			return f, nil
		}
		lastErr = err
		if !isStatus(err, http.StatusNotFound) {
			break
		}
		sleep(delay)
		delay *= 2
	}
	return nil, fmt.Errorf("copying file: %w", lastErr)
}

func (c *Copier) downloadUpload(ctx context.Context, f *drive.File, meta *drive.File) (*drive.File, error) {
	resp, err := c.Src.Files.Get(f.Id).SupportsAllDrives(true).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("downloading %q: %w", f.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	meta.MimeType = f.MimeType
	out, err := c.Dst.Files.Create(meta).
		Media(resp.Body, googleapi.ContentType(f.MimeType)).
		Fields("id,name,webViewLink").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("uploading %q: %w", f.Name, err)
	}
	return out, nil
}

func isStatus(err error, code int) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == code
}

func isCopyRefused(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != http.StatusForbidden {
		return false
	}
	for _, e := range gerr.Errors {
		if e.Reason == "cannotCopyFile" {
			return true
		}
	}
	return false
}

// escapeQuery escapes a value for a Drive query string literal.
func escapeQuery(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
