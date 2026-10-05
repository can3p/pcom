package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestRegenerateFeedToken_ReplacesOldToken(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)

	first := testutil.Must(repo.RegenerateFeedToken(ctx, db, user.ID))(t)
	owner := testutil.Must(repo.FeedTokenOwner(ctx, db, first.Token))(t)
	require.Equal(t, user.ID, owner.ID)

	second := testutil.Must(repo.RegenerateFeedToken(ctx, db, user.ID))(t)
	require.NotEqual(t, first.Token, second.Token)

	_, err := repo.FeedTokenOwner(ctx, db, first.Token)
	require.ErrorIs(t, err, sql.ErrNoRows)

	owner = testutil.Must(repo.FeedTokenOwner(ctx, db, second.Token))(t)
	require.Equal(t, user.ID, owner.ID)

	current := testutil.Must(repo.FeedTokenForUser(ctx, db, user.ID))(t)
	require.Equal(t, second.Token, current.Token)
	require.Equal(t, first.ID, second.ID)
}

func TestFeedTokenForUser_NoneIsNil(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)

	tok, err := repo.FeedTokenForUser(ctx, db, user.ID)
	require.NoError(t, err)
	require.Nil(t, tok)
}
