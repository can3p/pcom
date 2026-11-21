package notes

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ownNote is the one authorization of every method that names a note, so its
// table is tested here once rather than through each method.
func TestOwnNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	svc := New(store)

	owner := testutil.Must(factory.User(ctx, db))(t)
	other := testutil.Must(factory.User(ctx, db))(t)
	note := &model.Note{ID: uuid.NewString(), UserID: owner.ID, Body: "mine"}
	require.NoError(t, store.CreateNote(ctx, note))

	cases := []struct {
		name   string
		actor  *model.User
		noteID string
		want   error
	}{
		{"the owner", owner, note.ID, nil},
		{"anybody else is told it doesn't exist", other, note.ID, service.ErrNotFound},
		{"an anonymous actor needs to log in", nil, note.ID, service.ErrNeedsLogin},
		{"a note that doesn't exist", owner, uuid.NewString(), service.ErrNotFound},
	}

	for _, tc := range cases {
		for _, a := range []access{read, write} {
			err := store.Tx(ctx, func(tx *repo.Store) error {
				got, err := svc.ownNote(ctx, tx, tc.actor, tc.noteID, a)
				if err == nil {
					require.Equal(t, "mine", got.Body, "%s, access %d", tc.name, a)
				}

				return err
			})
			if tc.want == nil {
				require.NoError(t, err, "%s, access %d", tc.name, a)
			} else {
				require.ErrorIs(t, err, tc.want, "%s, access %d", tc.name, a)
			}
		}
	}
}
