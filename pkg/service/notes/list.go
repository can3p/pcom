package notes

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// Entry is a note of a list with the stack of the notes captured within
// the session gap of each other that it belongs to: StackID is the ID of
// the oldest note of the stack and StackSize the notes in it, whatever the
// filter. Used says whether the note is in a post.
type Entry = repo.NoteRow

// Filter picks the notes of a list. Tags and NotTags are the tags a note
// must and must not carry, from its tags and the #words of its text. Notes
// with a hidden tag are left out unless ShowHidden is set or the tag is one
// of Tags. Inbox is the notes without tags, Unused the ones in no post,
// Query a case-insensitive search in the body. Cursor is the Next of the
// previous page. StackID narrows the list to one stack, named by the ID of
// its oldest note, which is Entry.StackID: it holds as long as no older note
// joins the stack, and then the old ID is a stack of its own or is not found.
// A stack view takes no other filter, so it shows every note of the stack.
type Filter struct {
	StackID    string
	Tags       []string
	NotTags    []string
	ShowHidden bool
	SourceID   string
	Unused     bool
	Inbox      bool
	Query      string
	Cursor     string
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

	if f.StackID != "" {
		return s.listStack(ctx, actor, f)
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
		UserID:     actor.ID,
		Tags:       tags,
		NotTags:    notTags,
		ShowHidden: f.ShowHidden,
		SourceID:   f.SourceID,
		Unused:     f.Unused,
		Inbox:      f.Inbox,
		Query:      strings.TrimSpace(f.Query),
		Gap:        s.limits.SessionGap,
		BeforeAt:   at,
		BeforeID:   id,
		Limit:      s.pageSize + 1,
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

// listStack lists the notes of one stack. They are the newest notes at or
// below the stack's newest note, up to the next stack, so the page starts
// after that note and stops where the stack's ID changes.
func (s *Service) listStack(ctx context.Context, actor *model.User, f Filter) (*Page, error) {
	if _, err := uuid.Parse(f.StackID); err != nil {
		return nil, service.ErrNotFound
	}

	newest, _, err := s.store.NoteStackNewest(ctx, actor.ID, s.limits.SessionGap, f.StackID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	// strictly after the newest note: a microsecond later, any ID
	at, id := newest.Add(time.Microsecond), uuid.Max.String()

	if f.Cursor != "" {
		if at, id, err = parseCursor(f.Cursor); err != nil {
			return nil, err
		}
	}

	rows, err := s.store.ListNotes(ctx, repo.NoteFilter{
		UserID: actor.ID, ShowHidden: true, Gap: s.limits.SessionGap,
		BeforeAt: at, BeforeID: id, Limit: s.pageSize + 1,
	})
	if err != nil {
		return nil, err
	}

	var entries []Entry

	for _, r := range rows {
		if r.StackID != f.StackID {
			break
		}

		entries = append(entries, r)
	}

	page := &Page{Entries: entries}

	if len(entries) > s.pageSize {
		page.Entries = entries[:s.pageSize]
		page.Next = cursorOf(page.Entries[s.pageSize-1].Note)
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
