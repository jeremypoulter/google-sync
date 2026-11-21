// Package gmailcopy copies Gmail messages from the old account to the new one
// with messages.import, keeping labels and skipping messages that are already
// there (matched by their RFC 822 Message-ID).
package gmailcopy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"
	"sync"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

// Options control how a message is copied.
type Options struct {
	// LabelPrefix is put in front of every user label name in the new
	// account, e.g. "Old/" turns "Receipts" into "Old/Receipts". Empty keeps
	// names as they are.
	LabelPrefix string
}

// Result reports what happened to one message.
type Result struct {
	SourceID  string
	MessageID string // RFC 822 Message-ID header
	DestID    string
	Skipped   bool // already present in the destination
	Labels    []string
}

// Copier copies messages between two Gmail services. It caches label
// lookups, so reuse one Copier for many messages.
type Copier struct {
	Src, Dst *gmail.Service
	Opt      Options

	mu        sync.Mutex
	srcLabels map[string]*gmail.Label // id -> label
	dstByName map[string]string       // name -> id
}

// systemLabels exist with the same IDs in every mailbox and can be set on
// import. DRAFT and CHAT cannot be applied with messages.import.
var systemLabels = map[string]bool{
	"INBOX": true, "SENT": true, "STARRED": true, "IMPORTANT": true, "UNREAD": true,
	"TRASH": true, "SPAM": true,
	"CATEGORY_PERSONAL": true, "CATEGORY_SOCIAL": true, "CATEGORY_PROMOTIONS": true,
	"CATEGORY_UPDATES": true, "CATEGORY_FORUMS": true,
}

// Copy copies one message by its source Gmail ID.
func (c *Copier) Copy(ctx context.Context, srcID string) (*Result, error) {
	msg, err := c.Src.Users.Messages.Get("me", srcID).Format("raw").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("reading source message %s: %w", srcID, err)
	}
	raw, err := decodeRaw(msg.Raw)
	if err != nil {
		return nil, fmt.Errorf("decoding source message %s: %w", srcID, err)
	}
	res := &Result{SourceID: srcID, MessageID: MessageIDHeader(raw)}

	if res.MessageID != "" {
		existing, err := c.findByMessageID(ctx, res.MessageID)
		if err != nil {
			return nil, err
		}
		if existing != "" {
			res.DestID, res.Skipped = existing, true
			return res, nil
		}
	}

	labelIDs, names, err := c.mapLabels(ctx, msg.LabelIds)
	if err != nil {
		return nil, err
	}
	res.Labels = names

	// Always use the media upload path: it handles messages up to Gmail's
	// size limit, unlike the 5 MB inline raw field.
	imported, err := c.Dst.Users.Messages.Import("me", &gmail.Message{LabelIds: labelIDs}).
		InternalDateSource("dateHeader").
		NeverMarkSpam(true).
		ProcessForCalendar(false).
		Media(bytes.NewReader(raw), googleapi.ContentType("message/rfc822")).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("importing message %s: %w", srcID, err)
	}
	res.DestID = imported.Id
	return res, nil
}

// findByMessageID returns the destination ID of a message with this
// Message-ID, or "" if there is none.
func (c *Copier) findByMessageID(ctx context.Context, messageID string) (string, error) {
	resp, err := c.Dst.Users.Messages.List("me").
		Q(RFC822Query(messageID)).
		IncludeSpamTrash(true).
		MaxResults(1).
		Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("checking destination for %s: %w", messageID, err)
	}
	if len(resp.Messages) > 0 {
		return resp.Messages[0].Id, nil
	}
	return "", nil
}

// mapLabels turns source label IDs into destination label IDs, creating user
// labels in the destination as needed.
func (c *Copier) mapLabels(ctx context.Context, srcIDs []string) (ids, names []string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLabels(ctx); err != nil {
		return nil, nil, err
	}
	for _, id := range srcIDs {
		if systemLabels[id] {
			ids = append(ids, id)
			names = append(names, id)
			continue
		}
		l, ok := c.srcLabels[id]
		if !ok || l.Type == "system" {
			continue // DRAFT, CHAT, or an unknown system label
		}
		name := c.Opt.LabelPrefix + l.Name
		dstID, ok := c.dstByName[name]
		if !ok {
			created, err := c.Dst.Users.Labels.Create("me", &gmail.Label{
				Name:                  name,
				LabelListVisibility:   "labelShow",
				MessageListVisibility: "show",
			}).Context(ctx).Do()
			if err != nil {
				return nil, nil, fmt.Errorf("creating label %q: %w", name, err)
			}
			dstID = created.Id
			c.dstByName[name] = dstID
		}
		ids = append(ids, dstID)
		names = append(names, name)
	}
	return ids, names, nil
}

func (c *Copier) loadLabels(ctx context.Context) error {
	if c.srcLabels != nil {
		return nil
	}
	src, err := c.Src.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("listing source labels: %w", err)
	}
	dst, err := c.Dst.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("listing destination labels: %w", err)
	}
	c.srcLabels = map[string]*gmail.Label{}
	for _, l := range src.Labels {
		c.srcLabels[l.Id] = l
	}
	c.dstByName = map[string]string{}
	for _, l := range dst.Labels {
		c.dstByName[l.Name] = l.Id
	}
	return nil
}

// MessageIDHeader returns the Message-ID header of a raw RFC 822 message
// (angle brackets included, as Gmail search expects), or "".
func MessageIDHeader(raw []byte) string {
	r := bufio.NewReader(bytes.NewReader(raw))
	m, err := mail.ReadMessage(r)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(m.Header.Get("Message-Id"))
}

// RFC822Query builds a Gmail search for an exact Message-ID.
func RFC822Query(messageID string) string {
	return "rfc822msgid:" + strings.Trim(messageID, "<> \t")
}

func decodeRaw(s string) ([]byte, error) {
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}
