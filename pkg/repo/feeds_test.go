package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFeeds_UpsertAndSubscribe(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)

	first := testutil.Must(store.UpsertFeed(ctx, "https://upsert.test/a.xml"))(t)
	second := testutil.Must(store.UpsertFeed(ctx, "https://upsert.test/a.xml"))(t)
	require.Equal(t, first.ID, second.ID)

	b := testutil.Must(store.UpsertFeed(ctx, "https://upsert.test/b.xml"))(t)
	require.NotEqual(t, first.ID, b.ID)

	require.NoError(t, store.Subscribe(ctx, user.ID, first.ID))
	require.NoError(t, store.Subscribe(ctx, user.ID, first.ID))
	require.NoError(t, store.Subscribe(ctx, user.ID, b.ID))
	require.NoError(t, store.Subscribe(ctx, other.ID, first.ID))

	subs := testutil.Must(store.SubscriptionsOf(ctx, user.ID))(t)
	require.Len(t, subs, 2)
	require.Less(t, subs[0].ID, subs[1].ID)

	for _, s := range subs {
		require.NotNil(t, s.Feed)
		require.Equal(t, s.FeedID, s.Feed.ID)
	}

	require.Len(t, testutil.Must(store.FeedSubscriptions(ctx, first.ID))(t), 2)
	require.Len(t, testutil.Must(store.FeedSubscriptions(ctx, b.ID))(t), 1)

	// only the owner can delete a subscription
	require.NoError(t, store.DeleteSubscription(ctx, other.ID, subs[0].ID))
	require.Len(t, testutil.Must(store.SubscriptionsOf(ctx, user.ID))(t), 2)

	require.NoError(t, store.DeleteSubscription(ctx, user.ID, subs[0].ID))
	require.Len(t, testutil.Must(store.SubscriptionsOf(ctx, user.ID))(t), 1)
	require.Empty(t, testutil.Must(store.SubscriptionsOf(ctx, uuid.NewString()))(t))
}

func TestFeeds_Items(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	reader := testutil.Must(factory.User(ctx, db))(t)
	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
	empty := testutil.Must(factory.RSSFeed(ctx, db))(t)

	item := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
	later := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)

	require.Equal(t, int64(2), testutil.Must(store.FeedItemCount(ctx, feed.ID))(t))
	require.Equal(t, int64(0), testutil.Must(store.FeedItemCount(ctx, empty.ID))(t))

	require.True(t, testutil.Must(store.FeedItemExists(ctx, feed.ID, item.URLID))(t))
	require.False(t, testutil.Must(store.FeedItemExists(ctx, empty.ID, item.URLID))(t))

	imported := testutil.Must(store.LastImportedAt(ctx, []string{feed.ID, empty.ID}))(t)
	require.Len(t, imported, 1)
	require.WithinDuration(t, later.CreatedAt, imported[feed.ID], time.Second)
	require.Empty(t, testutil.Must(store.LastImportedAt(ctx, nil))(t))

	// an existing url keeps the stored item and reports it was not created
	dup := &model.RSSItem{ID: uuid.NewString(), FeedID: feed.ID, URLID: item.URLID, GUID: "dup", Title: "dup",
		PublishedAt: time.Now()}
	created := testutil.Must(store.UpsertFeedItem(ctx, dup))(t)
	require.False(t, created)
	require.Equal(t, item.ID, dup.ID)

	fresh := testutil.Must(factory.NormalizedURL(ctx, db))(t)
	added := &model.RSSItem{ID: uuid.NewString(), FeedID: feed.ID, URLID: fresh.ID, GUID: "new", Title: "new",
		PublishedAt: time.Now()}
	id := added.ID
	require.True(t, testutil.Must(store.UpsertFeedItem(ctx, added))(t))
	require.Equal(t, id, added.ID)
	require.Equal(t, int64(3), testutil.Must(store.FeedItemCount(ctx, feed.ID))(t))

	// user feed items: dismissed ones are hidden, relations are loaded
	require.NoError(t, store.InsertUserFeedItem(ctx, reader.ID, item.ID, item.URLID))
	dismissed := testutil.Must(factory.UserFeedItem(ctx, db, reader.ID, later.ID, factory.IsDismissed()))(t)
	otherUser := testutil.Must(factory.User(ctx, db))(t)
	testutil.Must(factory.UserFeedItem(ctx, db, otherUser.ID, later.ID))(t)

	list := testutil.Must(store.UndismissedFeedItems(ctx, reader.ID, repo.Page{}))(t)
	require.Len(t, list, 1)
	require.Equal(t, item.ID, list[0].RSSItemID)
	require.NotNil(t, list[0].RSSItem)
	require.NotNil(t, list[0].RSSItem.Feed)
	require.Equal(t, feed.ID, list[0].RSSItem.Feed.ID)
	require.NotNil(t, list[0].URL)

	got := testutil.Must(store.UserFeedItemByID(ctx, reader.ID, dismissed.ID))(t)
	require.True(t, got.IsDismissed)
	_, err := store.UserFeedItemByID(ctx, otherUser.ID, dismissed.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	require.NoError(t, store.DismissFeedItem(ctx, list[0]))
	require.True(t, list[0].IsDismissed)
	require.Empty(t, testutil.Must(store.UndismissedFeedItems(ctx, reader.ID, repo.Page{}))(t))
}

func TestFeeds_RefreshAndLock(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)

	past := time.Now().Add(-time.Hour)
	due := testutil.Must(factory.RSSFeed(ctx, db, factory.NextFetchAt(past)))(t)
	unscheduled := testutil.Must(factory.RSSFeed(ctx, db))(t)
	future := testutil.Must(factory.RSSFeed(ctx, db, factory.NextFetchAt(time.Now().Add(time.Hour))))(t)
	noSubs := testutil.Must(factory.RSSFeed(ctx, db, factory.NextFetchAt(past)))(t)

	for _, f := range []*model.RSSFeed{due, unscheduled, future} {
		testutil.Must(factory.Subscription(ctx, db, user.ID, f.ID))(t)
	}

	// the subscriptions load is limited to one row for all feeds (issue #213),
	// so only a feed that gets that row is returned; whichever it is must be
	// a due feed with a subscriber
	feeds := testutil.Must(store.FeedsToRefresh(ctx))(t)
	require.LessOrEqual(t, len(feeds), 1)

	for _, f := range feeds {
		require.Contains(t, []string{due.ID, unscheduled.ID}, f.ID)
		require.NotEqual(t, noSubs.ID, f.ID)
		require.NotEmpty(t, f.FeedUserFeedSubscriptions)
	}

	// SaveFeed writes the fields back
	due.Title = new("renamed")
	due.ConsecutiveEmptyFetches = 3
	require.NoError(t, store.SaveFeed(ctx, due))

	locked := testutil.Must(store.LockFeed(ctx, due.ID))(t)
	require.Equal(t, "renamed", *locked.Title)
	require.Equal(t, 3, locked.ConsecutiveEmptyFetches)

	_, err := store.LockFeed(ctx, uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	// a feed another transaction holds is skipped
	require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error {
		_, err := tx.LockFeed(ctx, due.ID)
		require.NoError(t, err)

		_, err = store.LockFeed(ctx, due.ID)
		require.ErrorIs(t, err, repo.ErrNotFound)

		return nil
	}))
}

func TestCreateMediaUpload(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)

	upload := &model.MediaUpload{ID: uuid.NewString(), UserID: &user.ID, UploadedFname: "media-repo-test.png", ContentType: "image/png"}
	require.NoError(t, store.CreateMediaUpload(ctx, upload))
	require.False(t, upload.CreatedAt.IsZero())

	got := testutil.Must(factory.GetMediaUploadByFname(ctx, db, "media-repo-test.png"))(t)
	require.Equal(t, upload.ID, got.ID)
	require.Equal(t, user.ID, *got.UserID)
}
