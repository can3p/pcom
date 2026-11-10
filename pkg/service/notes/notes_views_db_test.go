package notes_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

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
