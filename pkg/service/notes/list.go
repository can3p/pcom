package notes

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// Entry is a note of a list; Used says whether the note is in a post.
type Entry = repo.NoteRow

// Filter picks the notes of a list. Tags and NotTags are the tags a note
// must and must not carry, from its tags and the #words of its text, and
// Query a case-insensitive search in the body: the search box gives all
// three (ParseSearch). Inbox is the notes without tags, Unused the ones in
// no post. Cursor is the Next of the previous page.
type Filter struct {
	Tags     []string
	NotTags  []string
	SourceID string
	Unused   bool
	Inbox    bool
	Query    string
	Cursor   string
}

// Page is a page of a list, newest capture first.
type Page struct {
	Entries []Entry
	// Next is the cursor of the next page, empty on the last one.
	Next string
}

// List returns a page of the actor's notes.
func (s *Service) List(ctx context.Context, actor *model.User, f Filter) (*Page, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	tags, err := s.cleanTags(f.Tags)
	if err != nil {
		return nil, err
	}

	notTags, err := s.cleanTags(f.NotTags)
	if err != nil {
		return nil, err
	}

	if f.SourceID != "" {
		if _, err := s.ownSource(ctx, s.store, actor, f.SourceID); err != nil {
			return nil, err
		}
	}

	at, id, err := parseCursor(f.Cursor)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.ListNotes(ctx, repo.NoteFilter{
		UserID:   actor.ID,
		Tags:     tags,
		NotTags:  notTags,
		SourceID: f.SourceID,
		Unused:   f.Unused,
		Inbox:    f.Inbox,
		Query:    strings.TrimSpace(f.Query),
		BeforeAt: at,
		BeforeID: id,
		Limit:    s.pageSize + 1,
	})
	if err != nil {
		return nil, err
	}

	page := &Page{Entries: rows}

	if len(rows) > s.pageSize {
		page.Entries = rows[:s.pageSize]
		last := page.Entries[s.pageSize-1].Note
		page.Next = cursorOf(last)
	}

	return page, nil
}

// InboxCount is how many of the actor's notes have no tag.
func (s *Service) InboxCount(ctx context.Context, actor *model.User) (int, error) {
	if actor == nil {
		return 0, service.ErrNeedsLogin
	}

	return s.store.NoteInboxCount(ctx, actor.ID)
}

// cursorOf is the cursor right after a note: its capture time and ID.
func cursorOf(n *model.Note) string {
	raw := fmt.Sprintf("%d|%s", n.CapturedAt.UnixMicro(), n.ID)

	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes a cursor; an empty one is the first page.
func parseCursor(c string) (time.Time, string, error) {
	if c == "" {
		return time.Time{}, "", nil
	}

	invalid := service.Invalid("cursor", "This page link is not valid.")

	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", invalid
	}

	micros, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, "", invalid
	}

	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, "", invalid
	}

	if _, err := uuid.Parse(id); err != nil {
		return time.Time{}, "", invalid
	}

	return time.UnixMicro(n).UTC(), id, nil
}
