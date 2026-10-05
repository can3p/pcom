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
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func postIDs(ps []*model.Post) []string {
	return lo.Map(ps, func(p *model.Post, _ int) string { return p.ID })
}

func TestPublishedPostsOfUsers_KeepsTheVisibilityGrouping(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	all := testutil.Must(factory.User(ctx, db))(t)
	some := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)

	allDirect := testutil.Must(factory.Post(ctx, db, all.ID, factory.Published()))(t)
	testutil.Must(factory.Post(ctx, db, all.ID))(t) // draft
	somePublic := testutil.Must(factory.Post(ctx, db, some.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)
	testutil.Must(factory.Post(ctx, db, some.ID, factory.Published()))(t) // direct_only
	testutil.Must(factory.Post(ctx, db, other.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)
	testutil.Must(factory.PostStat(ctx, db, allDirect.ID))(t)

	public := []model.PostVisibility{model.PostVisibilityPublic}

	got, err := store.PublishedPostsOfUsers(ctx, []string{all.ID}, []string{some.ID}, public, repo.Page{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{allDirect.ID, somePublic.ID}, postIDs(got))

	for _, p := range got {
		require.Equal(t, p.UserID, p.User.ID)
		require.Nil(t, p.URL)

		if p.ID == allDirect.ID {
			require.NotNil(t, p.PostStat)
		} else {
			require.Nil(t, p.PostStat)
		}
	}

	got, err = store.PublishedPostsOfUsers(ctx, []string{all.ID}, nil, nil, repo.Page{})
	require.NoError(t, err)
	require.Equal(t, []string{allDirect.ID}, postIDs(got))

	got, err = store.PublishedPostsOfUsers(ctx, nil, []string{some.ID}, nil, repo.Page{})
	require.NoError(t, err)
	require.Empty(t, got, "no visibility lets nothing of someOf through")

	got, err = store.PublishedPostsOfUsers(ctx, nil, []string{some.ID}, []model.PostVisibility{}, repo.Page{})
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = store.PublishedPostsOfUsers(ctx, []string{}, []string{}, public, repo.Page{})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPublishedPostsOf_FiltersByVisibility(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	url := testutil.Must(store.StoreURL(ctx, "https://example.com/a"))(t)

	direct := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published(), factory.WithURL(url.ID)))(t)
	public := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)
	testutil.Must(factory.Post(ctx, db, author.ID))(t)
	testutil.Must(factory.Post(ctx, db, other.ID, factory.Published()))(t)
	testutil.Must(factory.PostStat(ctx, db, direct.ID))(t)

	got, err := store.PublishedPostsOf(ctx, author.ID, nil, false, repo.Page{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{direct.ID, public.ID}, postIDs(got))

	for _, p := range got {
		require.Nil(t, p.PostStat, "stats are not loaded")
		require.Equal(t, author.ID, p.User.ID)

		if p.ID == direct.ID {
			require.Equal(t, url.URL, p.URL.URL)
		} else {
			require.Nil(t, p.URL)
		}
	}

	got, err = store.PublishedPostsOf(ctx, author.ID, []model.PostVisibility{model.PostVisibilityPublic}, true, repo.Page{})
	require.NoError(t, err)
	require.Equal(t, []string{public.ID}, postIDs(got))

	got, err = store.PublishedPostsOf(ctx, author.ID, []model.PostVisibility{}, true, repo.Page{})
	require.NoError(t, err)
	require.Empty(t, got, "an empty visibility list matches nothing")

	got, err = store.PublishedPostsOf(ctx, author.ID, nil, true, repo.Page{})
	require.NoError(t, err)

	for _, p := range got {
		require.Equal(t, p.ID == direct.ID, p.PostStat != nil)
	}
}

func TestPublishedPostsByProfile_FiltersByAuthorProfile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	open := testutil.Must(factory.User(ctx, db, factory.WithVisibility(model.ProfileVisibilityPublic)))(t)
	closed := testutil.Must(factory.User(ctx, db, factory.WithVisibility(model.ProfileVisibilityRegisteredUsers)))(t)

	openPublic := testutil.Must(factory.Post(ctx, db, open.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)
	testutil.Must(factory.Post(ctx, db, open.ID, factory.Published()))(t)
	testutil.Must(factory.Post(ctx, db, open.ID, factory.Visibility(model.PostVisibilityPublic)))(t) // draft
	closedPublic := testutil.Must(factory.Post(ctx, db, closed.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)

	got, err := store.PublishedPostsByProfile(ctx, model.PostVisibilityPublic, []model.ProfileVisibility{model.ProfileVisibilityPublic}, repo.Page{})
	require.NoError(t, err)
	require.Equal(t, []string{openPublic.ID}, postIDs(got))
	require.Equal(t, open.ID, got[0].User.ID)

	got, err = store.PublishedPostsByProfile(ctx, model.PostVisibilityPublic,
		[]model.ProfileVisibility{model.ProfileVisibilityPublic, model.ProfileVisibilityRegisteredUsers}, repo.Page{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{openPublic.ID, closedPublic.ID}, postIDs(got))

	got, err = store.PublishedPostsByProfile(ctx, model.PostVisibilityPublic, nil, repo.Page{})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPostsOfOrAmong_KeepsTheGrouping(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	me := testutil.Must(factory.User(ctx, db))(t)
	author := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)

	myDraft := testutil.Must(factory.Post(ctx, db, me.ID))(t)
	listed := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)
	testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)
	strangers := testutil.Must(factory.Post(ctx, db, stranger.ID, factory.Published()))(t)

	got, err := store.PostsOfOrAmong(ctx, me.ID, []string{author.ID}, []string{listed.ID, strangers.ID})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{myDraft.ID, listed.ID}, postIDs(got))

	for _, p := range got {
		require.Equal(t, p.UserID, p.User.ID)
	}

	got, err = store.PostsOfOrAmong(ctx, me.ID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, []string{myDraft.ID}, postIDs(got))
}

func TestPostsByAuthor_PagesByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	old := time.Now().Add(-time.Hour).UTC()
	first := testutil.Must(factory.Post(ctx, db, author.ID, factory.PostUpdatedAt(old)))(t)
	second := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	third := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	testutil.Must(factory.Post(ctx, db, testutil.Must(factory.User(ctx, db))(t).ID))(t)

	got, err := store.PostsByAuthor(ctx, repo.PostsPage{AuthorID: author.ID, Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []string{third.ID, second.ID}, postIDs(got))

	got, err = store.PostsByAuthor(ctx, repo.PostsPage{AuthorID: author.ID, Limit: 5, Cursor: second.ID})
	require.NoError(t, err)
	require.Equal(t, []string{first.ID}, postIDs(got))

	got, err = store.PostsByAuthor(ctx, repo.PostsPage{AuthorID: author.ID, Limit: 5, UpdatedSince: old.Add(time.Minute)})
	require.NoError(t, err)
	require.Equal(t, []string{third.ID, second.ID}, postIDs(got))

	got, err = store.PostsByAuthor(ctx, repo.PostsPage{AuthorID: author.ID})
	require.NoError(t, err)
	require.Empty(t, got, "a zero limit matches nothing")
}

func TestPostLookups(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	url := testutil.Must(store.StoreURL(ctx, "https://example.com/x"))(t)
	draft := testutil.Must(factory.Post(ctx, db, author.ID, factory.WithURL(url.ID), factory.PostUpdatedAt(time.Now().Add(-time.Hour))))(t)
	published := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)
	draft2 := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	testutil.Must(factory.PostStat(ctx, db, draft.ID))(t)

	p, err := store.PostForEdit(ctx, draft.ID)
	require.NoError(t, err)
	require.Equal(t, author.ID, p.User.ID)
	require.Equal(t, draft.ID, p.PostStat.PostID)
	require.Equal(t, url.ID, p.URL.ID)

	p, err = store.PostToRead(ctx, published.ID)
	require.NoError(t, err)
	require.Nil(t, p.URL)
	require.Nil(t, p.PostStat)

	_, err = store.PostToRead(ctx, other.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	_, err = store.PostByID(ctx, other.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	_, err = store.OwnPost(ctx, draft.ID, other.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	p, err = store.OwnPost(ctx, draft.ID, author.ID)
	require.NoError(t, err)
	require.Equal(t, draft.ID, p.ID)

	err = store.Tx(ctx, func(tx *repo.Store) error {
		_, err := tx.OwnPostForUpdate(ctx, published.ID, author.ID, true)
		require.ErrorIs(t, err, repo.ErrNotFound)

		_, err = tx.OwnPostForUpdate(ctx, published.ID, other.ID, false)
		require.ErrorIs(t, err, repo.ErrNotFound)

		p, err := tx.OwnPostForUpdate(ctx, published.ID, author.ID, false)
		require.NoError(t, err)
		require.Equal(t, published.ID, p.ID)

		return nil
	})
	require.NoError(t, err)

	got, err := store.OwnPostsByIDs(ctx, author.ID, []string{draft.ID, published.ID, other.ID})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{draft.ID, published.ID}, postIDs(got))

	got, err = store.OwnPostsByIDs(ctx, author.ID, nil)
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = store.PostsForExport(ctx, author.ID, "")
	require.NoError(t, err)
	require.Len(t, got, 3)

	got, err = store.PostsForExport(ctx, author.ID, draft.ID)
	require.NoError(t, err)
	require.Equal(t, []string{draft.ID}, postIDs(got))
	require.Equal(t, url.URL, got[0].URL.URL)

	got, err = store.DraftPosts(ctx, author.ID)
	require.NoError(t, err)
	require.Equal(t, []string{draft2.ID, draft.ID}, postIDs(got))
}

func TestPostWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	author := testutil.Must(factory.User(ctx, db))(t)
	reader := testutil.Must(factory.User(ctx, db))(t)

	post := &model.Post{ID: uuid.NewString(), UserID: author.ID, Body: "b", VisibilityRadius: model.PostVisibilityPublic}
	require.NoError(t, store.InsertPost(ctx, post))
	require.NotNil(t, post.CreatedAt)
	created := *post.CreatedAt

	post.Body = "changed"
	require.NoError(t, store.UpdatePost(ctx, post))
	require.True(t, post.UpdatedAt.After(created))

	read := testutil.Must(store.PostByID(ctx, post.ID))(t)
	require.Equal(t, "changed", read.Body)

	testutil.Must(factory.Comment(ctx, db, post.ID, reader.ID))(t)
	testutil.Must(factory.PostStat(ctx, db, post.ID))(t)
	testutil.Must(factory.PostShare(ctx, db, post.ID))(t)

	require.NoError(t, store.DeletePost(ctx, post.ID))
	_, err := store.PostByID(ctx, post.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestStoreURL_KeepsTheKnownRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.Using(testdb.New(t).DB)

	first := testutil.Must(store.StoreURL(ctx, "https://example.com/page"))(t)
	again := testutil.Must(store.StoreURL(ctx, "https://example.com/page"))(t)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.URL, again.URL)

	other := testutil.Must(store.StoreURL(ctx, "https://example.com/other"))(t)
	require.NotEqual(t, first.ID, other.ID)
}

func TestUploadsByName(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.Using(db)

	owner := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	mine := testutil.Must(factory.MediaUpload(ctx, db, owner.ID))(t)
	theirs := testutil.Must(factory.MediaUpload(ctx, db, other.ID))(t)

	got, err := store.UploadsByName(ctx, owner.ID, []string{mine.UploadedFname, theirs.UploadedFname})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, mine.ID, got[0].ID)

	got, err = store.UploadsByName(ctx, owner.ID, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
