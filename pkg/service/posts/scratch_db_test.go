package posts_test

import (
	"context"
	"database/sql"
	"strings"
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

// padSave is a save of a new post's text by a tab that last saw revision rev.
func padSave(body string, rev int, action posts.Action) posts.SaveInput {
	return posts.SaveInput{Body: body, Visibility: model.PostVisibilityDirectOnly, Action: action, ScratchRevision: rev}
}

// seedPad writes the author's plain pad twice, as two autosaves of another
// tab, so it holds "theirs" at revision 2.
func seedPad(t *testing.T, ctx context.Context, s *posts.Service, author *model.User) {
	t.Helper()

	testutil.Must(s.Save(ctx, author, padSave("first", 0, posts.ActionAutosave)))(t)
	saved := testutil.Must(s.Save(ctx, author, padSave("theirs", 1, posts.ActionAutosave)))(t)
	require.Equal(t, 2, saved.Scratch.Revision)
}

func padOf(t *testing.T, ctx context.Context, db *sql.DB, userID, promptID string) *model.PostScratch {
	t.Helper()

	return testutil.Must(repo.New(db).ScratchOf(ctx, userID, promptID))(t)
}

// One row per row of the conflict table: what a save of this tab does to the
// pad and to the author's posts, at the revision it last saw.
func TestSave_ScratchRevisions(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	cases := []struct {
		name         string
		seeded       bool // another tab wrote "theirs" at revision 2
		rev          int
		action       posts.Action
		wantConflict bool
		wantPad      string
		wantPost     bool
	}{
		{name: "autosave, revision matches", seeded: true, rev: 2, action: posts.ActionAutosave, wantPad: "mine"},
		{name: "autosave, stale", seeded: true, rev: 1, action: posts.ActionAutosave, wantConflict: true, wantPad: "theirs"},
		{name: "first autosave, no pad yet", rev: 0, action: posts.ActionAutosave, wantPad: "mine"},
		{name: "first autosave, another tab inserted first", seeded: true, rev: 0, action: posts.ActionAutosave, wantConflict: true, wantPad: "theirs"},
		{name: "save as draft, revision matches", seeded: true, rev: 2, action: posts.ActionSavePost, wantPad: "", wantPost: true},
		{name: "save as draft, stale", seeded: true, rev: 1, action: posts.ActionSavePost, wantPad: "theirs", wantPost: true},
		{name: "publish, revision matches", seeded: true, rev: 2, action: posts.ActionPublish, wantPad: "", wantPost: true},
		{name: "discard, revision matches", seeded: true, rev: 2, action: posts.ActionDelete, wantPad: ""},
		{name: "discard, stale", seeded: true, rev: 1, action: posts.ActionDelete, wantConflict: true, wantPad: "theirs"},
		{name: "keep mine, stale", seeded: true, rev: 1, action: posts.ActionTakeOver, wantPad: "mine"},
		{name: "keep mine, no pad yet", rev: 0, action: posts.ActionTakeOver, wantPad: "mine"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, db))(t)
			s := svcWith(db, fakequeue.New())

			if tc.seeded {
				seedPad(t, ctx, s, author)
			}

			before := 0
			if tc.seeded {
				before = padOf(t, ctx, db, author.ID, "").Revision
			}

			saved, err := s.Save(ctx, author, padSave("mine", tc.rev, tc.action))

			pad := padOf(t, ctx, db, author.ID, "")
			require.Equal(t, tc.wantPad, pad.Body)
			list := testutil.Must(factory.ListPosts(ctx, db, author.ID))(t)

			if tc.wantConflict {
				var conflict *posts.ScratchConflict
				require.ErrorAs(t, err, &conflict)
				require.ErrorIs(t, err, service.ErrConflict)
				require.WithinDuration(t, pad.UpdatedAt, conflict.At, time.Millisecond)
				require.Empty(t, conflict.PromotedPostID)
				require.Equal(t, before, pad.Revision)
				require.Empty(t, list)

				return
			}

			require.NoError(t, err)

			if tc.wantPad == "theirs" {
				require.Equal(t, before, pad.Revision)
			} else {
				require.Equal(t, before+1, pad.Revision)
			}

			if !tc.wantPost {
				require.Empty(t, list)
				require.Nil(t, saved.Post)
				require.Nil(t, pad.PromotedPostID)

				if tc.action == posts.ActionDelete {
					require.True(t, saved.Deleted)
				} else {
					require.Equal(t, pad.Revision, saved.Scratch.Revision)
				}

				return
			}

			require.Len(t, list, 1)
			require.Equal(t, "mine", list[0].Body)
			require.Equal(t, tc.action == posts.ActionPublish, list[0].PublishedAt != nil)
			require.True(t, saved.Created)

			if tc.wantPad == "" {
				require.Equal(t, list[0].ID, *pad.PromotedPostID)
			} else {
				require.Nil(t, pad.PromotedPostID)
			}
		})
	}
}

// A tab whose pad another tab saved as a draft or discarded is told where its
// text went, and a later write of the pad no longer points at that post.
func TestSave_ScratchStaleTab(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	// promoted is an author whose pad another tab saved as a draft at
	// revision 2, so it is empty at revision 3.
	promoted := func(t *testing.T, s *posts.Service) (*model.User, *model.Post) {
		t.Helper()

		author := testutil.Must(factory.User(ctx, db))(t)
		seedPad(t, ctx, s, author)
		saved := testutil.Must(s.Save(ctx, author, padSave("theirs", 2, posts.ActionSavePost)))(t)

		return author, saved.Post
	}

	conflictOf := func(t *testing.T, err error) *posts.ScratchConflict {
		t.Helper()

		var conflict *posts.ScratchConflict
		require.ErrorAs(t, err, &conflict)

		return conflict
	}

	t.Run("saved as a draft", func(t *testing.T) {
		t.Parallel()

		s := svcWith(db, fakequeue.New())
		author, post := promoted(t, s)

		_, err := s.Save(ctx, author, padSave("mine", 2, posts.ActionAutosave))
		require.Equal(t, post.ID, conflictOf(t, err).PromotedPostID)

		pad := testutil.Must(s.Scratch(ctx, author, ""))(t)
		require.True(t, pad.IsEmpty())

		testutil.Must(s.Save(ctx, author, padSave("a new post", pad.Revision, posts.ActionAutosave)))(t)
		pad = padOf(t, ctx, db, author.ID, "")
		require.Equal(t, "a new post", pad.Body)
		require.Nil(t, pad.PromotedPostID)
	})

	t.Run("discarded after a draft", func(t *testing.T) {
		t.Parallel()

		s := svcWith(db, fakequeue.New())
		author, _ := promoted(t, s)
		testutil.Must(s.Save(ctx, author, padSave("", 3, posts.ActionDelete)))(t)

		_, err := s.Save(ctx, author, padSave("mine", 2, posts.ActionAutosave))
		require.Empty(t, conflictOf(t, err).PromotedPostID)
		require.Nil(t, padOf(t, ctx, db, author.ID, "").PromotedPostID)

		testutil.Must(s.Save(ctx, author, padSave("a new post", 4, posts.ActionAutosave)))(t)
		pad := padOf(t, ctx, db, author.ID, "")
		require.Equal(t, "a new post", pad.Body)
		require.Nil(t, pad.PromotedPostID)
	})

	t.Run("kept mine after a draft", func(t *testing.T) {
		t.Parallel()

		s := svcWith(db, fakequeue.New())
		author, _ := promoted(t, s)

		testutil.Must(s.Save(ctx, author, padSave("mine", 2, posts.ActionTakeOver)))(t)
		pad := padOf(t, ctx, db, author.ID, "")
		require.Equal(t, "mine", pad.Body)
		require.Nil(t, pad.PromotedPostID)
	})
}

// A prompt's pad sits beside the plain one, and the prompt is linked to the
// post only when the pad is published.
func TestSave_ScratchPerPrompt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, fakequeue.New())
	author := testutil.Must(factory.User(ctx, db))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)
	stranger := testutil.Must(factory.User(ctx, db))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, author.ID))(t)
	foreign := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, stranger.ID))(t)

	plain := padSave("plain text", 0, posts.ActionAutosave)
	answer := padSave("the answer", 0, posts.ActionAutosave)
	answer.PromptID = prompt.ID

	testutil.Must(s.Save(ctx, author, plain))(t)
	testutil.Must(s.Save(ctx, author, answer))(t)

	require.Equal(t, "plain text", testutil.Must(s.Scratch(ctx, author, ""))(t).Body)
	require.Equal(t, "the answer", testutil.Must(s.Scratch(ctx, author, prompt.ID))(t).Body)
	// a prompt addressed to somebody else is ignored, as on the write page
	require.Equal(t, "plain text", testutil.Must(s.Scratch(ctx, author, foreign.ID))(t).Body)
	require.Nil(t, testutil.Must(factory.GetPostPrompt(ctx, db, prompt.ID))(t).PostID)

	// a second tab's first autosave of the prompt's pad loses to the first
	_, err := s.Save(ctx, author, answer)
	var conflict *posts.ScratchConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "the answer", padOf(t, ctx, db, author.ID, prompt.ID).Body)
	plainPad := padOf(t, ctx, db, author.ID, "")
	require.Equal(t, "plain text", plainPad.Body)
	require.Equal(t, 1, plainPad.Revision)

	answer.Action, answer.ScratchRevision = posts.ActionPublish, 1
	saved := testutil.Must(s.Save(ctx, author, answer))(t)

	require.Equal(t, saved.Post.ID, *testutil.Must(factory.GetPostPrompt(ctx, db, prompt.ID))(t).PostID)
	require.True(t, testutil.Must(s.Scratch(ctx, author, prompt.ID))(t).IsEmpty())
	require.Equal(t, "plain text", testutil.Must(s.Scratch(ctx, author, ""))(t).Body)
}

// Autosaving a new post, however often, makes no post and tells nobody.
func TestSave_ScratchAutosaveMakesNoPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	q := fakequeue.New()
	s := svcWith(db, q)
	author := testutil.Must(factory.User(ctx, db))(t)

	rev := 0
	for _, body := range []string{"a", "a b", "a b c"} {
		in := padSave(body, rev, posts.ActionAutosave)
		in.Visibility = model.PostVisibilityPublic
		in.NotifySubscribers = new(true)
		rev = testutil.Must(s.Save(ctx, author, in))(t).Scratch.Revision
	}

	require.Equal(t, 3, rev)
	require.Empty(t, testutil.Must(factory.ListPosts(ctx, db, author.ID))(t))
	require.Empty(t, q.Inserted())

	pad := padOf(t, ctx, db, author.ID, "")
	require.Equal(t, "a b c", pad.Body)
	require.Equal(t, 3, pad.Revision)
}

func TestSave_ScratchRefusesInvalid(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, fakequeue.New())
	author := testutil.Must(factory.User(ctx, db))(t)
	post := testutil.Must(factory.Post(ctx, db, author.ID))(t)

	takeOver := padSave("mine", 0, posts.ActionTakeOver)
	takeOver.PostID = post.ID
	long := padSave(strings.Repeat("x", posts.DefaultPostBodyMaxLength+1), 0, posts.ActionAutosave)

	for field, in := range map[string]posts.SaveInput{"save_action": takeOver, "body": long} {
		_, err := s.Save(ctx, author, in)

		var ve *service.ValidationError
		require.ErrorAs(t, err, &ve, field)
		require.Equal(t, field, ve.Field)
		require.Nil(t, testutil.Must(s.Scratch(ctx, author, ""))(t), field)
	}
}

// The pad goes with its author and with its prompt.
func TestScratch_DeletedWithOwner(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	s := svcWith(db, fakequeue.New())

	gone := testutil.Must(factory.User(ctx, db, factory.Unconfirmed()))(t)
	testutil.Must(s.Save(ctx, gone, padSave("text", 0, posts.ActionAutosave)))(t)
	require.Equal(t, "text", padOf(t, ctx, db, gone.ID, "").Body)
	require.True(t, testutil.Must(store.DeleteUnconfirmedUser(ctx, gone.ID))(t))
	_, err := store.ScratchOf(ctx, gone.ID, "")
	require.ErrorIs(t, err, repo.ErrNotFound)

	author := testutil.Must(factory.User(ctx, db))(t)
	asker := testutil.Must(factory.User(ctx, db))(t)
	linked := testutil.Must(factory.Post(ctx, db, author.ID))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, db, asker.ID, author.ID, factory.WithPost(linked.ID)))(t)
	answer := padSave("text", 0, posts.ActionAutosave)
	answer.PromptID = prompt.ID
	testutil.Must(s.Save(ctx, author, answer))(t)
	testutil.Must(s.Save(ctx, author, padSave("plain text", 0, posts.ActionAutosave)))(t)
	require.Equal(t, "text", padOf(t, ctx, db, author.ID, prompt.ID).Body)

	// deleting the post a prompt is linked to deletes the prompt
	require.NoError(t, store.DeletePost(ctx, linked.ID))
	_, err = store.ScratchOf(ctx, author.ID, prompt.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	require.Equal(t, "plain text", padOf(t, ctx, db, author.ID, "").Body)
}

// The pad keeps the notes inserted into the new post, without repeats, on
// every write, and Scratch loads them.
func TestSave_ScratchKeepsNoteIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	s := svcWith(db, fakequeue.New())
	author := testutil.Must(factory.User(ctx, db))(t)
	a, b := "0190a5b2-0000-7000-8000-000000000001", "0190a5b2-0000-7000-8000-000000000002"

	in := padSave("text", 0, posts.ActionAutosave)
	in.NoteIDs = []string{a, "", a}
	testutil.Must(s.Save(ctx, author, in))(t)
	require.Equal(t, []string{a}, testutil.Must(s.Scratch(ctx, author, ""))(t).NoteIDs)

	in.NoteIDs, in.ScratchRevision = []string{a, b}, 1
	testutil.Must(s.Save(ctx, author, in))(t)
	require.Equal(t, []string{a, b}, testutil.Must(s.Scratch(ctx, author, ""))(t).NoteIDs)

	in.NoteIDs, in.Action = []string{b}, posts.ActionTakeOver
	testutil.Must(s.Save(ctx, author, in))(t)
	require.Equal(t, []string{b}, testutil.Must(s.Scratch(ctx, author, ""))(t).NoteIDs)

	in.NoteIDs, in.ScratchRevision, in.Action = []string{"not-a-uuid"}, 3, posts.ActionAutosave
	_, err := s.Save(ctx, author, in)

	var ve *service.ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "note_ids", ve.Field)
	require.Equal(t, []string{b}, padOf(t, ctx, db, author.ID, "").NoteIDs)
}
