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

func TestQuoteSource(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	idle := notes.DefaultLimits.FocusIdle

	_, err := w.svc.QuoteSource(w.ctx, nil)
	require.ErrorIs(t, err, service.ErrNeedsLogin)

	got, err := w.svc.QuoteSource(w.ctx, w.me)
	require.NoError(t, err)
	require.Nil(t, got, "no notes")

	old := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	recent := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	theirs := testutil.Must(factory.NoteSource(w.ctx, w.db, w.other.ID))(t)

	w.note(t, w.me, factory.FromSource(old.ID), factory.CapturedAt(w.now.Add(-idle/2)))
	w.note(t, w.me, factory.FromSource(recent.ID), factory.CapturedAt(w.now.Add(-time.Minute)))
	w.note(t, w.me) // a note without a source doesn't reset the rule
	w.note(t, w.other, factory.FromSource(theirs.ID), factory.CapturedAt(w.now))

	got, err = w.svc.QuoteSource(w.ctx, w.me)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, recent.ID, got.ID, "the previous quote's source, not somebody else's")

	w.now = w.now.Add(idle)

	got, err = w.svc.QuoteSource(w.ctx, w.me)
	require.NoError(t, err)
	require.Nil(t, got, "idle for too long")
}
