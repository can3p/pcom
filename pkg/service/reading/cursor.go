package reading

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// DefaultPageSize is how many items a page of a list holds (the feed,
// explore, the index page and a journal) unless WithLimits sets it.
const DefaultPageSize = 30

// DefaultRSSLimit caps every RSS output, which isn't paged, unless
// WithLimits sets it.
const DefaultRSSLimit = 50

// Cursor is where a page starts: right after the item with this sort time,
// kind and ID. Clients get it as an opaque string.
type Cursor struct {
	Time time.Time
	Kind string
	ID   string
}

// String encodes the cursor, base64url.
func (c Cursor) String() string {
	raw := fmt.Sprintf("%d|%s|%s", c.Time.UnixMicro(), c.Kind, c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseCursor decodes a cursor. An empty string is the first page, the zero
// Cursor; one that doesn't parse is service.Invalid.
func ParseCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}

	invalid := service.Invalid("cursor", "This page link is not valid.")

	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, invalid
	}

	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return Cursor{}, invalid
	}

	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || micros == 0 {
		return Cursor{}, invalid
	}

	switch parts[1] {
	case repo.KindComment, repo.KindPost, repo.KindRSSItem:
	default:
		return Cursor{}, invalid
	}

	if _, err := uuid.Parse(parts[2]); err != nil {
		return Cursor{}, invalid
	}

	return Cursor{Time: time.UnixMicro(micros).UTC(), Kind: parts[1], ID: parts[2]}, nil
}

// page is the repository page after the cursor, one row longer than limit so
// that the service can tell whether another page follows.
func (c Cursor) page(limit int) repo.Page {
	return repo.Page{Before: c.Time, BeforeKind: c.Kind, BeforeID: c.ID, Limit: limit + 1}
}

// cursorOf is the cursor right after an item.
func cursorOf(item *FeedItem) Cursor {
	switch {
	case item.Post != nil:
		return Cursor{Time: item.AddedToFeedAt(), Kind: repo.KindPost, ID: item.Post.ID}
	case item.FeedItem != nil:
		return Cursor{Time: item.AddedToFeedAt(), Kind: repo.KindRSSItem, ID: item.FeedItem.ID}
	default:
		return Cursor{Time: item.AddedToFeedAt(), Kind: repo.KindComment, ID: item.Comment.ID}
	}
}

// compareItems orders feed items newest first, then by kind and ID, the
// order of the repository's pages.
func compareItems(a, b *FeedItem) int {
	ca, cb := cursorOf(a), cursorOf(b)

	if c := cb.Time.Compare(ca.Time); c != 0 {
		return c
	}

	if c := strings.Compare(cb.Kind, ca.Kind); c != 0 {
		return c
	}

	return strings.Compare(cb.ID, ca.ID)
}

// cut keeps the first limit items of a sorted list and returns the cursor
// of the next page, empty when there is none.
func cut[T any](items []T, limit int, cursor func(T) Cursor) ([]T, string) {
	if len(items) <= limit {
		return items, ""
	}

	items = items[:limit]

	return items, cursor(items[limit-1]).String()
}
