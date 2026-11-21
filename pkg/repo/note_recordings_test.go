package repo_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// A note's recording is a row of note_recordings: an image upload can't be
// one, and deleting the recording leaves the note without one.
func TestNoteRecordings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)

	image := testutil.Must(factory.MediaUpload(ctx, db, user.ID))(t)
	_, err := factory.Note(ctx, db, user.ID, factory.WithRecording(image.ID))

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "an image upload is not a recording")
	require.Equal(t, "23503", pgErr.Code, "a foreign key violation")
	require.Equal(t, "notes_recording_id_fkey", pgErr.ConstraintName)

	// the same id as a recording is one
	rec := &model.NoteRecording{ID: image.ID, UserID: user.ID, Fname: "audio/" + image.ID + ".ogg", ContentType: "audio/ogg", SizeBytes: 7}
	require.NoError(t, store.CreateNoteRecording(ctx, rec))
	require.False(t, rec.CreatedAt.IsZero())

	note := testutil.Must(factory.Note(ctx, db, user.ID, factory.WithRecording(rec.ID)))(t)
	require.True(t, testutil.Must(store.NoteUsesRecording(ctx, rec.ID))(t))

	got := testutil.Must(store.NoteRecordingByID(ctx, rec.ID))(t)
	require.Equal(t, rec.Fname, got.Fname)
	require.Equal(t, int64(7), got.SizeBytes)

	require.NoError(t, store.DeleteNoteRecording(ctx, rec.ID))
	_, err = store.NoteRecordingByID(ctx, rec.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	require.Nil(t, testutil.Must(store.NoteByID(ctx, note.ID))(t).RecordingID, "the note loses its recording")
	require.False(t, testutil.Must(store.NoteUsesRecording(ctx, rec.ID))(t))
}
