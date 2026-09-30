package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStore_Tx(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	author := testutil.Must(factory.User(ctx, db))(t)
	errBoom := errors.New("boom")

	cases := []struct {
		name       string
		fn         func(tx *repo.Store, postID string) error
		wantErr    error
		wantShared bool
	}{
		{"commits when fn succeeds", func(tx *repo.Store, postID string) error {
			return tx.CreateShare(ctx, postID)
		}, nil, true},
		{"rolls back when fn fails", func(tx *repo.Store, postID string) error {
			require.NoError(t, tx.CreateShare(ctx, postID))
			return errBoom
		}, errBoom, false},
		{"a nested Tx joins the outer one and rolls back with it", func(tx *repo.Store, postID string) error {
			require.NoError(t, tx.Tx(ctx, func(inner *repo.Store) error { return inner.CreateShare(ctx, postID) }))
			return errBoom
		}, errBoom, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)

			err := store.Tx(ctx, func(tx *repo.Store) error { return tc.fn(tx, post.ID) })
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantShared, testutil.Must(factory.ShareExists(ctx, db, post.ID))(t))
		})
	}
}

func TestStore_NotFound(t *testing.T) {
	t.Parallel()

	store := repo.New(testdb.New(t).DB)

	_, err := store.ShareByID(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)

	_, err = store.PostByID(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, repo.ErrNotFound)
}
