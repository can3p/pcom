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

func TestCreateNoteImage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	user := testutil.Must(factory.User(ctx, db))(t)
	note := testutil.Must(factory.Note(ctx, db, user.ID))(t)
	first := testutil.Must(factory.NoteAudio(ctx, db, user.ID))(t)
	second := testutil.Must(factory.NoteAudio(ctx, db, user.ID))(t)

	// inserted in the opposite order to the positions
	require.NoError(t, store.CreateNoteImage(ctx, &model.NoteImage{NoteID: note.ID, UploadID: second.ID, Position: 1}))
	require.NoError(t, store.CreateNoteImage(ctx, &model.NoteImage{NoteID: note.ID, UploadID: first.ID, Position: 0}))

	got := testutil.Must(store.NoteWithRelations(ctx, note.ID))(t)
	require.Len(t, got.Images, 2)
	require.Equal(t, []string{first.ID, second.ID}, []string{got.Images[0].UploadID, got.Images[1].UploadID})
	require.Equal(t, first.UploadedFname, got.Images[0].Upload.UploadedFname)

	err := store.CreateNoteImage(ctx, &model.NoteImage{NoteID: note.ID, UploadID: first.ID, Position: 2})

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "an upload is in a note once")
	require.Equal(t, "23505", pgErr.Code, "a unique violation")
}
