package repo

import (
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// Page is one page of a list sorted newest first by a time, then by kind and
// ID, so that the order is total even across lists that are merged. A page
// holds what comes after the item (Before, BeforeKind, BeforeID); a zero
// Before is the first page. Limit caps the rows, 0 means no cap.
type Page struct {
	Before     time.Time
	BeforeKind string
	BeforeID   string
	Limit      int
}

// The kinds of the lists a feed merges. Kinds compare as strings and the
// merged order is descending, so on a tie it is rss, post, comment.
const (
	KindComment = "comment"
	KindPost    = "post"
	KindRSSItem = "rss"
)

// mods sorts a list of kind by timeCol and idCol, newest first, and keeps the
// rows that come after the page's item in the merged order.
func (p Page) mods(kind, timeCol, idCol string) []qm.QueryMod {
	m := []qm.QueryMod{qm.OrderBy(fmt.Sprintf("%s DESC, %s DESC", timeCol, idCol))}

	if p.Limit > 0 {
		m = append(m, qm.Limit(p.Limit))
	}

	switch {
	case p.Before.IsZero():
	case kind < p.BeforeKind:
		m = append(m, qm.Where(timeCol+" <= ?", p.Before))
	case kind > p.BeforeKind:
		m = append(m, qm.Where(timeCol+" < ?", p.Before))
	default:
		m = append(m, qm.Where(fmt.Sprintf("(%s, %s) < (?, ?)", timeCol, idCol), p.Before, p.BeforeID))
	}

	return m
}

// apply is mods for bun: it sorts q, a list of kind, by timeCol and idCol,
// newest first, and keeps the rows that come after the page's item in the
// merged order. Qualify the columns with ?TableAlias when q joins relations.
func (p Page) apply(q *bun.SelectQuery, kind, timeCol, idCol string) *bun.SelectQuery {
	q = q.OrderExpr(fmt.Sprintf("%s DESC, %s DESC", timeCol, idCol))

	if p.Limit > 0 {
		q = q.Limit(int64(p.Limit))
	}

	switch {
	case p.Before.IsZero():
	case kind < p.BeforeKind:
		q = q.Where(timeCol+" <= ?", p.Before)
	case kind > p.BeforeKind:
		q = q.Where(timeCol+" < ?", p.Before)
	default:
		q = q.Where(fmt.Sprintf("(%s, %s) < (?, ?)", timeCol, idCol), p.Before, p.BeforeID)
	}

	return q
}
