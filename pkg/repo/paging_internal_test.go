package repo

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

// TestPage_Apply pins the SQL Page.apply adds for each position in the
// merged order; the repository tests in paging_test.go check the rows.
func TestPage_Apply(t *testing.T) {
	t.Parallel()

	before := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	const order = `ORDER BY "post".published_at DESC, "post".id DESC`

	for name, tc := range map[string]struct {
		page Page
		want string
	}{
		"first page":       {Page{}, order},
		"limit":            {Page{Limit: 5}, order + " LIMIT 5"},
		"earlier kind":     {Page{Before: before, BeforeKind: KindRSSItem}, `WHERE ("post".published_at <= '2026-10-05 12:00:00+00:00') ` + order},
		"later kind":       {Page{Before: before, BeforeKind: KindComment}, `WHERE ("post".published_at < '2026-10-05 12:00:00+00:00') ` + order},
		"same kind, by id": {Page{Before: before, BeforeKind: KindPost, BeforeID: "p1"}, `WHERE (("post".published_at, "post".id) < ('2026-10-05 12:00:00+00:00', 'p1')) ` + order},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			q := builder.NewSelect().Model((*model.Post)(nil)).Column("id")
			got := tc.page.apply(q, KindPost, "?TableAlias.published_at", "?TableAlias.id").String()
			require.Equal(t, `SELECT "post"."id" FROM "posts" AS "post" `+tc.want, got)
		})
	}
}
