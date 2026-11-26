package posts_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// postSave is a save of a post's text by a tab that last saw revision rev.
func postSave(postID, body string, rev int, action posts.Action) posts.SaveInput {
	return posts.SaveInput{PostID: postID, Body: body, Visibility: model.PostVisibilityDirectOnly, Action: action, PostRevision: rev}
}

// One row per action: what a save of a published post does when this tab saw
// the post's revision, and when another tab wrote the post since.
func TestSave_PostRevisions(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	cases := []struct {
		name         string
		stale        bool
		action       posts.Action
		wantConflict bool
		wantBody     string
		wantDraft    bool
		wantCopy     bool
		wantDeleted  bool
	}{
		{name: "autosave", action: posts.ActionAutosave, wantBody: "mine"},
		{name: "autosave, stale", stale: true, action: posts.ActionAutosave, wantConflict: true, wantBody: "theirs"},
		{name: "save post, stale", stale: true, action: posts.ActionSavePost, wantConflict: true, wantBody: "theirs"},
		{name: "publish, stale", stale: true, action: posts.ActionPublish, wantConflict: true, wantBody: "theirs"},
		{name: "back to draft", action: posts.ActionMakeDraft, wantBody: "mine", wantDraft: true},
		{name: "back to draft, stale", stale: true, action: posts.ActionMakeDraft, wantConflict: true, wantBody: "theirs"},
		{name: "keep mine, stale", stale: true, action: posts.ActionTakeOver, wantBody: "mine"},
		{name: "save as a new draft, stale", stale: true, action: posts.ActionSaveCopy, wantBody: "theirs", wantCopy: true},
		{name: "delete, stale", stale: true, action: posts.ActionDelete, wantDeleted: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db))(t)
			post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published(), factory.WithBody("first")))(t)
			s := svcWith(db, fakequeue.New())

			// another tab saved "theirs" over revision 1
			theirs := testutil.Must(s.Save(ctx, author, postSave(post.ID, "theirs", 1, posts.ActionAutosave)))(t)
			require.Equal(t, 2, theirs.Post.Revision)

			rev := 2
			if tc.stale {
				rev = 1
			}

			saved, err := s.Save(ctx, author, postSave(post.ID, "mine", rev, tc.action))

			if tc.wantDeleted {
				require.NoError(t, err)
				require.True(t, saved.Deleted)
				require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, author.ID))(t))

				return
			}

			stored := testutil.Must(repo.New(db).OwnPost(ctx, post.ID, author.ID))(t)
			require.Equal(t, tc.wantBody, stored.Body)
			require.Equal(t, tc.wantDraft, stored.PublishedAt == nil)

			if tc.wantConflict {
				var conflict *posts.PostConflict
				require.ErrorAs(t, err, &conflict)
				require.ErrorIs(t, err, service.ErrConflict)
				require.WithinDuration(t, *stored.UpdatedAt, conflict.At, time.Millisecond)
				require.Equal(t, 2, stored.Revision)

				return
			}

			require.NoError(t, err)

			if tc.wantCopy {
				require.True(t, saved.Created)
				require.NotEqual(t, post.ID, saved.Post.ID)
				require.Nil(t, saved.Post.PublishedAt)
				require.Equal(t, "mine", saved.Post.Body)
				require.Equal(t, 1, saved.Post.Revision)
				require.Equal(t, 2, stored.Revision)

				return
			}

			require.Equal(t, 3, stored.Revision)
			require.Equal(t, 3, saved.Post.Revision)
		})
	}
}

// A save without a revision (the API) replaces the post whatever its
// revision, and still bumps it, so a tab that saw the old one loses.
func TestSave_PostWithoutRevision(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, fakequeue.New())
	author := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	testutil.Must(s.Save(ctx, author, postSave(post.ID, "api", 0, posts.ActionMakeDraft)))(t)

	_, err := s.Save(ctx, author, postSave(post.ID, "mine", 1, posts.ActionAutosave))
	require.ErrorIs(t, err, service.ErrConflict)
	require.Equal(t, "api", testutil.Must(repo.New(db).OwnPost(ctx, post.ID, author.ID))(t).Body)
}

// A copy is of the actor's own post.
func TestSave_CopyOfOthersPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, fakequeue.New())
	author := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	_, err := s.Save(ctx, other, postSave(post.ID, "mine", 1, posts.ActionSaveCopy))
	require.ErrorIs(t, err, service.ErrNotFound)
	require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, other.ID))(t))
}
