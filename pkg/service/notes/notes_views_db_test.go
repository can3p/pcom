package notes_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

func TestListStackShowsOnlyItsNotes(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithPageSize(2))
	base := w.now.Add(-48 * time.Hour)
	at := func(m int) factory.NoteOpt { return factory.CapturedAt(base.Add(time.Duration(m) * time.Minute)) }

	w.note(t, w.me, at(-300))
	a := w.note(t, w.me, at(0), factory.WithNoteTags("done"))
	b := w.note(t, w.me, at(5))
	c := w.note(t, w.me, at(10))
	after := w.note(t, w.me, at(300))
	theirs := w.note(t, w.other, at(1))

	require.NoError(t, w.svc.HideTag(w.ctx, w.me, "done"))

	// the stack ID is the oldest note's, the one the list reports
	p, err := w.svc.List(w.ctx, w.me, notes.Filter{})
	require.NoError(t, err)
	require.Equal(t, a.ID, p.Entries[1].StackID)

	first, err := w.svc.List(w.ctx, w.me, notes.Filter{StackID: a.ID})
	require.NoError(t, err)
	require.Equal(t, []string{c.ID, b.ID}, ids(first), "hidden tags don't hide a stack's notes; other stacks stay out")
	require.NotEmpty(t, first.Next)

	second, err := w.svc.List(w.ctx, w.me, notes.Filter{StackID: a.ID, Cursor: first.Next})
	require.NoError(t, err)
	require.Equal(t, []string{a.ID}, ids(second))
	require.Empty(t, second.Next)

	single, err := w.svc.List(w.ctx, w.me, notes.Filter{StackID: after.ID})
	require.NoError(t, err)
	require.Equal(t, []string{after.ID}, ids(single))

	for name, f := range map[string]notes.Filter{
		"another user's stack": {StackID: theirs.ID},
		"not a stack":          {StackID: c.ID},
		"not an id":            {StackID: "nope"},
	} {
		_, err := w.svc.List(w.ctx, w.me, f)
		require.ErrorIs(t, err, service.ErrNotFound, name)
	}

}

func TestInboxCount(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	w.note(t, w.me)
	w.note(t, w.me)
	w.note(t, w.me, factory.WithNoteTags("a"))
	w.note(t, w.me, factory.WithNoteTextTags("b"))
	w.note(t, w.other)

	n, err := w.svc.InboxCount(w.ctx, w.me)
	require.NoError(t, err)
	require.Equal(t, 2, n)

}

func TestPostLinks(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	note := w.note(t, w.me)
	theirs := w.note(t, w.other)

	n, err := w.svc.PostLinks(w.ctx, w.me, note.ID)
	require.NoError(t, err)
	require.Zero(t, n)

	for range 2 {
		post := testutil.Must(factory.Post(w.ctx, w.db, w.me.ID))(t)
		testutil.Must(factory.NotePostLink(w.ctx, w.db, note.ID, post.ID))(t)
	}

	n, err = w.svc.PostLinks(w.ctx, w.me, note.ID)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	_, err = w.svc.PostLinks(w.ctx, w.me, theirs.ID)
	require.ErrorIs(t, err, service.ErrNotFound)
}
