package notes_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

func (w *world) post(t *testing.T, u *model.User, opts ...factory.PostOpt) *model.Post {
	t.Helper()

	return testutil.Must(factory.Post(w.ctx, w.db, u.ID, opts...))(t)
}

// posts is the posts service on the world's store and queue.
func (w *world) posts() *posts.Service {
	store := repo.New(w.db)

	return posts.New(store, nil, posts.WithJobs(w.queue), posts.WithSubscriptions(subscriptions.New(store, subscriptions.WithJobs(w.queue))))
}

// links are the rows of note_post_links of a note, by post and kind.
func (w *world) links(t *testing.T, noteID string) []string {
	t.Helper()

	out := []string{}
	for _, l := range testutil.Must(repo.New(w.db).NoteLinks(w.ctx, noteID))(t) {
		out = append(out, l.PostID+"|"+string(l.How))
	}

	return out
}

type attached struct {
	id       string
	oneByOne bool
	viaTags  []string
}

func (w *world) forPost(t *testing.T, postID string) ([]attached, []string) {
	t.Helper()

	got := testutil.Must(w.svc.ForPost(w.ctx, w.me, postID))(t)
	out := []attached{}

	for _, n := range got.Notes {
		out = append(out, attached{n.Note.ID, n.Attached, n.ViaTags})
	}

	return out, got.Tags
}

func postIDs(posts []*model.Post) []string {
	out := []string{}
	for _, p := range posts {
		out = append(out, p.ID)
	}

	return out
}

func TestForPost_AttachedAndTaggedLessExcluded(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	at := func(m int) factory.NoteOpt { return factory.CapturedAt(w.now.Add(time.Duration(m) * time.Minute)) }

	post := w.post(t, w.me)
	both := w.note(t, w.me, at(0), factory.WithNoteTags("walden"), factory.WithNoteTextTags("walden"))
	byText := w.note(t, w.me, at(1), factory.WithNoteTextTags("pond", "walden"))
	one := w.note(t, w.me, at(2))
	excluded := w.note(t, w.me, at(3), factory.WithNoteTags("walden"))
	byHand := w.note(t, w.me, at(4))
	byEdit := w.note(t, w.me, at(5))
	w.note(t, w.other, at(6), factory.WithNoteTags("walden"))

	require.NoError(t, w.svc.Attach(w.ctx, w.me, one.ID, post.ID))
	require.NoError(t, w.svc.Attach(w.ctx, w.me, both.ID, post.ID))
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, excluded.ID, post.ID))
	require.Equal(t, 2, testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "#Walden"))(t))
	require.Equal(t, 1, testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "pond"))(t))

	notesOf, tags := w.forPost(t, post.ID)
	require.Equal(t, []string{"pond", "walden"}, tags)
	require.Equal(t, []attached{
		{one.ID, true, []string{}},
		{byText.ID, false, []string{"pond", "walden"}},
		{both.ID, true, []string{"walden"}},
	}, notesOf)

	// notes that gain the tag later join, by hand or by #word; attaching
	// takes an exclusion back
	require.NoError(t, w.svc.TagMany(w.ctx, w.me, []string{byHand.ID}, []string{"walden"}))
	testutil.Must(w.svc.Update(w.ctx, w.me, byEdit.ID, notes.Edit{Body: "by the #walden pond"}))(t)
	require.NoError(t, w.svc.Attach(w.ctx, w.me, excluded.ID, post.ID))

	notesOf, _ = w.forPost(t, post.ID)
	require.Equal(t, []attached{
		{byEdit.ID, false, []string{"walden"}},
		{byHand.ID, false, []string{"walden"}},
		{excluded.ID, true, []string{"walden"}},
		{one.ID, true, []string{}},
		{byText.ID, false, []string{"pond", "walden"}},
		{both.ID, true, []string{"walden"}},
	}, notesOf)

	// detaching drops a note attached one by one, which stays a note, and
	// keeps a tagged one; excluding drops a note, attached or not
	require.NoError(t, w.svc.Detach(w.ctx, w.me, one.ID, post.ID))
	require.NoError(t, w.svc.Detach(w.ctx, w.me, both.ID, post.ID))
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, excluded.ID, post.ID))
	require.NoError(t, w.svc.DetachTag(w.ctx, w.me, post.ID, "pond"))

	notesOf, tags = w.forPost(t, post.ID)
	require.Equal(t, []string{"walden"}, tags)
	require.Equal(t, []attached{
		{byEdit.ID, false, []string{"walden"}},
		{byHand.ID, false, []string{"walden"}},
		{byText.ID, false, []string{"walden"}},
		{both.ID, false, []string{"walden"}},
	}, notesOf)
	w.get(t, one.ID)
}

func TestPostsOf(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	note := w.note(t, w.me, factory.WithNoteTags("walden"))
	byTag := w.post(t, w.me)
	oneByOne := w.post(t, w.me, factory.Published())
	excluded := w.post(t, w.me)
	inserted := w.post(t, w.me)
	theirs := w.post(t, w.other)

	require.NoError(t, w.svc.Attach(w.ctx, w.me, note.ID, oneByOne.ID))
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, byTag.ID, "walden"))(t)
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, excluded.ID, "walden"))(t)
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, note.ID, excluded.ID))
	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, note.ID, inserted.ID))
	testutil.Must(w.svc.AttachTag(w.ctx, w.other, theirs.ID, "walden"))(t)

	// inserting a note into a post attaches it
	require.Equal(t, []string{inserted.ID, oneByOne.ID, byTag.ID}, postIDs(testutil.Must(w.svc.PostsOf(w.ctx, w.me, note.ID))(t)))
	byNote := testutil.Must(w.svc.PostsOfNotes(w.ctx, w.me, []string{note.ID}))(t)
	require.Equal(t, []string{inserted.ID, oneByOne.ID, byTag.ID}, postIDs(byNote[note.ID]))

	// deleting a post removes its links only
	require.NoError(t, w.posts().Delete(w.ctx, w.me, oneByOne.ID))
	require.Equal(t, []string{inserted.ID, byTag.ID}, postIDs(testutil.Must(w.svc.PostsOf(w.ctx, w.me, note.ID))(t)))
	require.ElementsMatch(t, []string{excluded.ID + "|excluded", inserted.ID + "|inserted", inserted.ID + "|attached"}, w.links(t, note.ID))

	// deleting the note removes its links and leaves the posts their tags
	require.NoError(t, w.svc.Delete(w.ctx, w.me, note.ID))
	require.Empty(t, w.links(t, note.ID))
	require.Equal(t, []string{"walden"}, testutil.Must(w.svc.ForPost(w.ctx, w.me, byTag.ID))(t).Tags)
}

// Publishing a draft and making it a draft again change none of its notes.
func TestForPost_KeptThroughPublishing(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)
	require.NoError(t, w.svc.Attach(w.ctx, w.me, w.note(t, w.me).ID, post.ID))
	w.note(t, w.me, factory.WithNoteTags("walden"))
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t)

	before := testutil.Must(w.svc.ForPost(w.ctx, w.me, post.ID))(t)
	require.Len(t, before.Notes, 2)

	for _, action := range []posts.Action{posts.ActionPublish, posts.ActionMakeDraft} {
		testutil.Must(w.posts().Save(w.ctx, w.me, posts.SaveInput{PostID: post.ID, Body: "a body", Visibility: model.PostVisibilityPublic, Action: action}))(t)
		require.Equal(t, before, testutil.Must(w.svc.ForPost(w.ctx, w.me, post.ID))(t), action)
	}
}

// Attaching is not using: the list's Used and PostLinks count inserted notes
// only, and a note can be inserted into a post and attached to it.
func TestAttach_DoesNotUseTheNote(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)
	attachedOnly := w.note(t, w.me, factory.WithNoteTags("walden"))
	insertedToo := w.note(t, w.me)

	require.NoError(t, w.svc.Attach(w.ctx, w.me, attachedOnly.ID, post.ID))
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t)
	require.NoError(t, w.svc.Attach(w.ctx, w.me, insertedToo.ID, post.ID))
	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, insertedToo.ID, post.ID))
	require.NoError(t, w.svc.Attach(w.ctx, w.me, insertedToo.ID, post.ID))

	used := map[string]bool{}
	for _, e := range testutil.Must(w.svc.List(w.ctx, w.me, notes.Filter{}))(t).Entries {
		used[e.Note.ID] = e.Used
	}

	require.Equal(t, map[string]bool{attachedOnly.ID: false, insertedToo.ID: true}, used)
	require.Zero(t, testutil.Must(w.svc.PostLinks(w.ctx, w.me, attachedOnly.ID))(t))
	require.Equal(t, 1, testutil.Must(w.svc.PostLinks(w.ctx, w.me, insertedToo.ID))(t))
	require.ElementsMatch(t, []string{post.ID + "|inserted", post.ID + "|attached"}, w.links(t, insertedToo.ID))

	notesOf, _ := w.forPost(t, post.ID)
	require.Contains(t, notesOf, attached{insertedToo.ID, true, []string{}})
}

// Every method needs the actor to own the note and the post it names.
func TestAttach_OwnerOnly(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	myNote := w.note(t, w.me, factory.WithNoteTags("walden"))
	myPost := w.post(t, w.me)
	theirNote := w.note(t, w.other)
	theirPost := w.post(t, w.other, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	_, _, err := factory.Connect(w.ctx, w.db, w.me.ID, w.other.ID)
	require.NoError(t, err)

	calls := map[string]func(actor *model.User, noteID, postID string) error{
		"attach":  func(a *model.User, n, p string) error { return w.svc.Attach(w.ctx, a, n, p) },
		"detach":  func(a *model.User, n, p string) error { return w.svc.Detach(w.ctx, a, n, p) },
		"exclude": func(a *model.User, n, p string) error { return w.svc.Exclude(w.ctx, a, n, p) },
		"attach tag": func(a *model.User, _, p string) error {
			_, err := w.svc.AttachTag(w.ctx, a, p, "walden")
			return err
		},
		"detach tag": func(a *model.User, _, p string) error { return w.svc.DetachTag(w.ctx, a, p, "walden") },
		"for post": func(a *model.User, _, p string) error {
			_, err := w.svc.ForPost(w.ctx, a, p)
			return err
		},
		"post": func(a *model.User, _, p string) error {
			_, err := w.svc.Post(w.ctx, a, p)
			return err
		},
		"posts of": func(a *model.User, n, _ string) error {
			_, err := w.svc.PostsOf(w.ctx, a, n)
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, call(nil, myNote.ID, myPost.ID), service.ErrNeedsLogin)
			require.ErrorIs(t, call(w.other, myNote.ID, myPost.ID), service.ErrNotFound)

			if name != "posts of" {
				require.ErrorIs(t, call(w.me, myNote.ID, theirPost.ID), service.ErrNotFound)
			}

			if name != "attach tag" && name != "detach tag" && name != "for post" && name != "post" {
				require.ErrorIs(t, call(w.me, theirNote.ID, myPost.ID), service.ErrNotFound)
			}

			require.NoError(t, call(w.me, myNote.ID, myPost.ID))
		})
	}

	require.Empty(t, w.links(t, theirNote.ID))
	require.NotContains(t, w.links(t, myNote.ID), theirPost.ID+"|attached")
	require.NotContains(t, w.links(t, myNote.ID), theirPost.ID+"|excluded")
	require.Empty(t, testutil.Must(repo.New(w.db).PostNoteTags(w.ctx, theirPost.ID))(t))
}

func TestAttachTag_RefusesABadTag(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)

	for _, tag := range []string{"", "#", "no spaces"} {
		t.Run(tag, func(t *testing.T) {
			_, err := w.svc.AttachTag(w.ctx, w.me, post.ID, tag)

			var invalid *service.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Equal(t, "tag", invalid.Field)
		})
	}

	require.Empty(t, testutil.Must(repo.New(w.db).PostNoteTags(w.ctx, post.ID))(t))
}

// Inserting a note into a post attaches it, taking back an exclusion;
// detaching removes the attachment and leaves the record of the insert.
func TestMarkUsed_AttachesTheNote(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)
	note := w.note(t, w.me, factory.WithNoteTags("walden"))
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t)
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, note.ID, post.ID))

	require.NoError(t, w.svc.MarkUsed(w.ctx, w.me, note.ID, post.ID))
	require.ElementsMatch(t, []string{post.ID + "|attached", post.ID + "|inserted"}, w.links(t, note.ID))

	require.NoError(t, w.svc.Detach(w.ctx, w.me, note.ID, post.ID))
	require.Equal(t, []string{post.ID + "|inserted"}, w.links(t, note.ID))
}

// A note excluded from a post stays out when the tag is attached again, and
// comes back when it is attached by hand.
func TestAttachTag_KeepsExclusions(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)
	note := w.note(t, w.me, factory.WithNoteTags("walden"))

	testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t)
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, note.ID, post.ID))
	require.NoError(t, w.svc.DetachTag(w.ctx, w.me, post.ID, "walden"))
	require.Zero(t, testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t))

	notesOf, _ := w.forPost(t, post.ID)
	require.Empty(t, notesOf)
}

// The notes list leaves out the notes that belong to a post unless asked
// for all of them, and filtered by a post it shows that post's notes whether
// or not the list hides them; it pages like the rest.
func TestList_ByPostAndHidingAttached(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithPageSize(1))
	at := func(m int) factory.NoteOpt { return factory.CapturedAt(w.now.Add(time.Duration(m) * time.Minute)) }
	post := w.post(t, w.me)
	other := w.post(t, w.me)
	loose := w.note(t, w.me, at(0))
	byHand := w.note(t, w.me, at(1))
	byTag := w.note(t, w.me, at(2), factory.WithNoteTags("walden"))
	hidden := w.note(t, w.me, at(3), factory.WithNoteTags("walden"))

	require.NoError(t, w.svc.Attach(w.ctx, w.me, byHand.ID, post.ID))
	testutil.Must(w.svc.AttachTag(w.ctx, w.me, post.ID, "walden"))(t)
	require.NoError(t, w.svc.Exclude(w.ctx, w.me, hidden.ID, post.ID))

	ids := func(f notes.Filter) []string {
		var out []string

		for {
			page := testutil.Must(w.svc.List(w.ctx, w.me, f))(t)
			for _, e := range page.Entries {
				out = append(out, e.Note.ID)
			}

			if page.Next == "" {
				return out
			}

			f.Cursor = page.Next
		}
	}

	require.Equal(t, []string{hidden.ID, byTag.ID, byHand.ID, loose.ID}, ids(notes.Filter{}))
	require.Equal(t, []string{hidden.ID, loose.ID}, ids(notes.Filter{HideAttached: true}))
	require.Equal(t, []string{byTag.ID, byHand.ID}, ids(notes.Filter{PostID: post.ID, HideAttached: true}))
	require.Equal(t, []string{byTag.ID}, ids(notes.Filter{PostID: post.ID, Tags: []string{"walden"}}))
	require.Empty(t, ids(notes.Filter{PostID: other.ID}))

	_, err := w.svc.List(w.ctx, w.other, notes.Filter{PostID: post.ID})
	require.ErrorIs(t, err, service.ErrNotFound)
}

// Attach to post offers the newest saved posts, as many as the setting says.
func TestAttachablePosts(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithLimits(notes.Limits{AttachPosts: 2}))
	oldest := w.post(t, w.me)
	middle := w.post(t, w.me, factory.Published())
	newest := w.post(t, w.me)
	w.post(t, w.other)

	require.Equal(t, []string{newest.ID, middle.ID}, postIDs(testutil.Must(w.svc.AttachablePosts(w.ctx, w.me))(t)))
	require.NotContains(t, postIDs(testutil.Must(w.svc.AttachablePosts(w.ctx, w.me))(t)), oldest.ID)

	_, err := w.svc.AttachablePosts(w.ctx, nil)
	require.ErrorIs(t, err, service.ErrNeedsLogin)
}

// The counts of the views by source follow the list beside them: without the
// notes in posts, with all of them, or with those of one post.
func TestSourcesOf_FollowTheListsScope(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	post := w.post(t, w.me)
	src := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	inPost := w.note(t, w.me, factory.FromSource(src.ID))
	w.note(t, w.me, factory.FromSource(src.ID))
	w.note(t, w.other, factory.FromSource(testutil.Must(factory.NoteSource(w.ctx, w.db, w.other.ID))(t).ID))
	require.NoError(t, w.svc.Attach(w.ctx, w.me, inPost.ID, post.ID))

	count := func(f notes.Filter) int {
		got := testutil.Must(w.svc.SourcesOf(w.ctx, w.me, f))(t)
		require.Len(t, got, 1)

		return got[0].Notes
	}

	require.Equal(t, 1, count(notes.Filter{HideAttached: true}))
	require.Equal(t, 2, count(notes.Filter{}))
	require.Equal(t, 1, count(notes.Filter{PostID: post.ID, HideAttached: true}))
	require.Zero(t, count(notes.Filter{PostID: w.post(t, w.me).ID}))
}
