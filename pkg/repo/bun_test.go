package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// These tests pin the bun behaviors the repositories rely on. Each is a rule
// in "Writing bun queries" in docs/architecture.md.

func insertUser(t *testing.T, q repo.Queries) *model.User {
	t.Helper()

	name := "u" + uuid.NewString()[:8]
	u := &model.User{
		ID:             uuid.NewString(),
		Email:          name + "@example.com",
		EmailCanonical: name + "@example.com",
		Username:       name,
		Timezone:       "UTC",
	}
	_, err := q.NewInsert().Model(u).Exec(context.Background())
	require.NoError(t, err)

	return u
}

func insertFeedWithSubscriber(t *testing.T, q repo.Queries) *model.RSSFeed {
	t.Helper()

	ctx := context.Background()
	u := insertUser(t, q)
	f := &model.RSSFeed{ID: uuid.NewString(), URL: "https://example.com/" + uuid.NewString()}
	_, err := q.NewInsert().Model(f).Exec(ctx)
	require.NoError(t, err)

	_, err = q.NewInsert().Model(&model.UserFeedSubscription{ID: uuid.NewString(), UserID: u.ID, FeedID: f.ID}).Exec(ctx)
	require.NoError(t, err)

	return f
}

func insertPost(t *testing.T, q repo.Queries, userID string) *model.Post {
	t.Helper()

	p := &model.Post{ID: uuid.NewString(), UserID: userID, Body: "body", VisibilityRadius: model.PostVisibilityPublic}
	_, err := q.NewInsert().Model(p).Exec(context.Background())
	require.NoError(t, err)

	return p
}

// A relation bun loads with a query of its own (has-many) runs on the same
// executor as the main query, so inside a transaction it sees the rows the
// transaction inserted.
func TestBun_HasManyRelationRunsInTheTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)

	err := store.Tx(ctx, func(tx *repo.Store) error {
		q := repo.Query(tx.Exec())
		f := insertFeedWithSubscriber(t, q)

		var got model.RSSFeed
		require.NoError(t, q.NewSelect().Model(&got).Relation("FeedUserFeedSubscriptions").
			Where("?TableAlias.id = ?", f.ID).Scan(ctx))
		require.Len(t, got.FeedUserFeedSubscriptions, 1)

		return nil
	})
	require.NoError(t, err)
}

// bun joins a belongs-to relation with LEFT JOIN, and Postgres refuses to
// lock the nullable side of an outer join: a lock with a joined relation
// names the main table.
func TestBun_ForUpdateWithAJoinedRelationNamesTheTable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := repo.Query(testdb.New(t).DB)
	p := insertPost(t, q, insertUser(t, q).ID)

	var got model.Post
	err := q.NewSelect().Model(&got).Relation("User").Where("?TableAlias.id = ?", p.ID).For("UPDATE").Scan(ctx)
	require.ErrorContains(t, err, "nullable side of an outer join")

	got = model.Post{}
	require.NoError(t, q.NewSelect().Model(&got).Relation("User").Where("?TableAlias.id = ?", p.ID).
		For("UPDATE OF ?TableAlias").Scan(ctx))
	require.Equal(t, p.UserID, got.User.ID)
}

// bun.List with an empty slice is valid SQL and matches no row.
func TestBun_ListWithAnEmptySliceMatchesNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := repo.Query(testdb.New(t).DB)
	insertPost(t, q, insertUser(t, q).ID)

	var posts []*model.Post
	require.NoError(t, q.NewSelect().Model(&posts).Where("user_id IN (?)", bun.List([]string{})).Scan(ctx))
	require.Empty(t, posts)

	require.NoError(t, q.NewSelect().Model(&posts).
		WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Where("user_id IN (?)", bun.List([]string{})).WhereOr("visibility_radius IN (?)", bun.List([]model.PostVisibility{model.PostVisibilityPublic}))
		}).Scan(ctx))
	require.Len(t, posts, 1, "an empty IN inside an OR leaves the other branch")
}

// A zero value in a column with a default inserts DEFAULT and the value
// comes back; created_at and updated_at are stamped by the model.
func TestBun_InsertReturnsDefaultsAndStampsTimes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := repo.Query(testdb.New(t).DB)

	before := time.Now().Add(-time.Second)
	u := insertUser(t, q)
	require.Equal(t, model.ProfileVisibilityRegisteredUsers, u.ProfileVisibility)
	require.NotNil(t, u.CreatedAt)
	require.True(t, u.CreatedAt.After(before))

	f := insertFeedWithSubscriber(t, q)
	require.True(t, f.CreatedAt.After(before), "a column without a default is stamped too")
	require.Equal(t, f.CreatedAt, f.UpdatedAt)

	created := f.CreatedAt
	f.Title = new("renamed")
	_, err := q.NewUpdate().Model(f).WherePK().Exec(ctx)
	require.NoError(t, err)
	require.True(t, f.UpdatedAt.After(created), "an update stamps updated_at")
	require.Equal(t, created, f.CreatedAt)
}

// Enums, nullable enums and jsonb survive a write and a read.
func TestBun_EnumsAndJSONRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := repo.Query(testdb.New(t).DB)

	reason := model.RSSFeedDisableReasonNoSubscribers
	f := &model.RSSFeed{ID: uuid.NewString(), URL: "https://example.com/feed", DisableReason: &reason}
	_, err := q.NewInsert().Model(f).Exec(ctx)
	require.NoError(t, err)

	var feed model.RSSFeed
	require.NoError(t, q.NewSelect().Model(&feed).Where("id = ?", f.ID).Scan(ctx))
	require.Equal(t, &reason, feed.DisableReason)

	mail := &model.OutgoingEmail{
		ID: uuid.NewString(), UniqueID: uuid.NewString(), EmailType: "test",
		Payload: []byte(`{"to": "a@example.com", "n": 1}`), Status: model.OutgoingEmailStatusNew, TryAt: time.Now(),
	}
	_, err = q.NewInsert().Model(mail).Exec(ctx)
	require.NoError(t, err)

	var got model.OutgoingEmail
	require.NoError(t, q.NewSelect().Model(&got).Where("id = ?", mail.ID).Scan(ctx))
	require.Equal(t, model.OutgoingEmailStatusNew, got.Status)
	require.JSONEq(t, string(mail.Payload), string(got.Payload))
}

// A limit on a has-many relation limits the one query that loads it for
// every parent, not each parent's rows.
func TestBun_LimitOnAHasManyRelationIsShared(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := repo.Query(testdb.New(t).DB)
	a, b := insertFeedWithSubscriber(t, q), insertFeedWithSubscriber(t, q)

	var feeds []*model.RSSFeed
	require.NoError(t, q.NewSelect().Model(&feeds).
		Relation("FeedUserFeedSubscriptions", func(sq *bun.SelectQuery) *bun.SelectQuery { return sq.Limit(1) }).
		Where("?TableAlias.id IN (?)", bun.List([]string{a.ID, b.ID})).Scan(ctx))
	require.Len(t, feeds, 2)
	require.Equal(t, 1, len(feeds[0].FeedUserFeedSubscriptions)+len(feeds[1].FeedUserFeedSubscriptions))
}

// A has-many relation is loaded by a query of its own, so its rows are
// locked by a For inside the relation's apply function.
func TestBun_ForInsideAHasManyRelationLocksItsRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	f := insertFeedWithSubscriber(t, repo.Query(db))

	err := repo.New(db).Tx(ctx, func(tx *repo.Store) error {
		var got model.RSSFeed
		require.NoError(t, repo.Query(tx.Exec()).NewSelect().Model(&got).
			Relation("FeedUserFeedSubscriptions", func(sq *bun.SelectQuery) *bun.SelectQuery { return sq.For("UPDATE") }).
			Where("?TableAlias.id = ?", f.ID).Scan(ctx))
		require.Len(t, got.FeedUserFeedSubscriptions, 1)

		var id string
		err := db.QueryRowContext(ctx, "SELECT id FROM user_feed_subscriptions WHERE id = $1 FOR UPDATE NOWAIT",
			got.FeedUserFeedSubscriptions[0].ID).Scan(&id)
		require.ErrorContains(t, err, "could not obtain lock")

		return nil
	})
	require.NoError(t, err)
}
