package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestAPIKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := accounts.New(repo.New(db), nil, accounts.WithClock(func() time.Time { return now }))
	user := newUser(t, ctx, db)
	other := newUser(t, ctx, db)

	foreign := testutil.Must(svc.CreateAPIKey(ctx, other, "theirs", model.APIKeyScopeFull))(t)
	full := testutil.Must(svc.CreateAPIKey(ctx, user, " laptop ", ""))(t)
	phone := testutil.Must(svc.CreateAPIKey(ctx, user, "iPhone", model.APIKeyScopeNotes))(t)

	require.NotEmpty(t, full.APIKey)
	require.NotEqual(t, full.APIKey, phone.APIKey)
	require.Equal(t, model.HashAPIKey(full.APIKey), full.KeyHash, "the key is stored as a hash")
	require.Equal(t, "laptop", full.Name)
	require.Equal(t, model.APIKeyScopeFull, full.Scope, "no scope is a full key")

	_, err := svc.CreateAPIKey(ctx, user, "x", "admin")
	require.ErrorAs(t, err, new(*service.ValidationError))

	keys := testutil.Must(svc.APIKeys(ctx, user))(t)
	require.Len(t, keys, 2)
	require.Equal(t, []string{full.ID, phone.ID}, []string{keys[0].ID, keys[1].ID}, "oldest first")
	require.Empty(t, keys[0].APIKey, "a listed key carries no plain key")
	require.Nil(t, keys[0].LastUsedAt)

	used := testutil.Must(svc.UseAPIKey(ctx, phone.APIKey))(t)
	require.Equal(t, user.ID, used.User.ID)
	require.Equal(t, model.APIKeyScopeNotes, used.Scope)

	keys = testutil.Must(svc.APIKeys(ctx, user))(t)
	require.Nil(t, keys[0].LastUsedAt)
	require.NotNil(t, keys[1].LastUsedAt)
	require.True(t, now.Equal(*keys[1].LastUsedAt), "the use is recorded at the clock's time")

	require.ErrorIs(t, svc.RevokeAPIKey(ctx, user, foreign.ID), service.ErrNotFound, "another user's key")
	require.ErrorIs(t, svc.RevokeAPIKey(ctx, user, "not-a-uuid"), service.ErrNotFound)
	require.NoError(t, svc.RevokeAPIKey(ctx, user, phone.ID))
	require.ErrorIs(t, svc.RevokeAPIKey(ctx, user, phone.ID), service.ErrNotFound, "already revoked")

	_, err = svc.UseAPIKey(ctx, phone.APIKey)
	require.ErrorIs(t, err, service.ErrNotFound, "a revoked key stops working")
	_, err = svc.UseAPIKey(ctx, "unknown")
	require.ErrorIs(t, err, service.ErrNotFound)
	require.Equal(t, other.ID, testutil.Must(svc.UseAPIKey(ctx, foreign.APIKey))(t).User.ID, "another user's key is untouched")

	_, err = svc.CreateAPIKey(ctx, nil, "x", "")
	require.ErrorIs(t, err, service.ErrNeedsLogin)
	_, err = svc.APIKeys(ctx, nil)
	require.ErrorIs(t, err, service.ErrNeedsLogin)
	require.ErrorIs(t, svc.RevokeAPIKey(ctx, nil, full.ID), service.ErrNeedsLogin)
}
