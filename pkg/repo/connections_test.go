package repo_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type graphFixture struct {
	ctx   context.Context
	store *repo.Store
	users []*model.User
}

func newGraphFixture(t *testing.T, n int) *graphFixture {
	t.Helper()

	ctx := context.Background()
	db := testdb.New(t).DB
	f := &graphFixture{ctx: ctx, store: repo.New(db)}

	for range n {
		u, err := factory.User(ctx, db)
		require.NoError(t, err)
		f.users = append(f.users, u)
	}

	return f
}

func (f *graphFixture) connect(t *testing.T, a, b int) *model.UserConnection {
	t.Helper()

	c1, c2, err := f.store.CreateConnection(f.ctx, f.users[a].ID, f.users[b].ID)
	require.NoError(t, err)
	assert.Equal(t, f.users[b].ID, c1.User2ID)
	assert.Equal(t, f.users[a].ID, c2.User2ID)
	assert.False(t, c1.CreatedAt.IsZero())

	return c1
}

// A chain 0-1-2 plus an unrelated 3: the graph queries follow the edges only.
func TestGraph_Queries(t *testing.T) {
	t.Parallel()

	f := newGraphFixture(t, 4)
	ctx, store, u := f.ctx, f.store, f.users
	f.connect(t, 0, 1)
	f.connect(t, 1, 2)

	ids, err := store.ConnectedUserIDs(ctx, u[1].ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{u[0].ID, u[2].ID}, ids)

	ids, err = store.ConnectedUserIDs(ctx, u[3].ID)
	require.NoError(t, err)
	assert.NotNil(t, ids)
	assert.Empty(t, ids)

	hops, err := store.TwoHops(ctx, u[0].ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []*repo.Hop{
		{UserID: u[0].ID, ViaUserID: u[1].ID},
		{UserID: u[2].ID, ViaUserID: u[1].ID},
	}, hops)

	ok, err := store.Connected(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = store.Connected(ctx, u[0].ID, u[2].ID)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = store.ConnectedThroughOne(ctx, u[0].ID, u[2].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = store.ConnectedThroughOne(ctx, u[0].ID, u[3].ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

// Deleting a connection removes both rows, the grant and the mediation
// request with its mediators, and leaves other connections alone.
func TestDeleteConnectionsBetween(t *testing.T) {
	t.Parallel()

	f := newGraphFixture(t, 4)
	ctx, store, u := f.ctx, f.store, f.users

	// 0 and 1 connected through a grant; 0 and 2 through a mediated request.
	require.NoError(t, store.CreateGrant(ctx, u[1].ID, u[0].ID))
	grant, err := store.LockOpenGrant(ctx, u[1].ID, u[0].ID)
	require.NoError(t, err)
	c01 := f.connect(t, 0, 1)
	require.NoError(t, store.UseGrant(ctx, grant, c01.ID))

	require.NoError(t, store.CreateMediationRequest(ctx, u[0].ID, u[2].ID, ""))
	req, err := store.MediationRequestBetween(ctx, u[0].ID, u[2].ID)
	require.NoError(t, err)
	require.NoError(t, store.AddMediatorDecision(ctx, req.ID, u[3].ID, model.ConnectionMediationDecisionSigned, "ok"))
	c02 := f.connect(t, 0, 2)
	require.NoError(t, store.RecordTargetDecision(ctx, req, model.ConnectionRequestDecisionApproved, c02.ID, ""))

	f.connect(t, 2, 3)

	require.NoError(t, store.DeleteConnectionsBetween(ctx, u[1].ID, u[0].ID))
	ok, err := store.Connected(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.Connected(ctx, u[1].ID, u[0].ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.GrantExists(ctx, u[1].ID, u[0].ID)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, store.DeleteConnectionsBetween(ctx, u[0].ID, u[2].ID))
	ok, err = store.MediationRequestExists(ctx, u[0].ID, u[2].ID)
	require.NoError(t, err)
	assert.False(t, ok)

	ids, err := store.ConnectedUserIDs(ctx, u[2].ID)
	require.NoError(t, err)
	assert.Equal(t, []string{u[3].ID}, ids)

	// Without a connection it does nothing.
	require.NoError(t, store.DeleteConnectionsBetween(ctx, u[0].ID, u[3].ID))
}

func TestWhitelistGrants(t *testing.T) {
	t.Parallel()

	f := newGraphFixture(t, 3)
	ctx, store, u := f.ctx, f.store, f.users

	got, err := store.WhitelistTarget(ctx, u[1].Username)
	require.NoError(t, err)
	assert.Equal(t, u[1].ID, got.ID)

	_, err = store.WhitelistTarget(ctx, "nobody-here")
	require.ErrorIs(t, err, repo.ErrNotFound)

	require.NoError(t, store.CreateGrant(ctx, u[0].ID, u[1].ID))
	require.NoError(t, store.CreateGrant(ctx, u[0].ID, u[2].ID))
	require.NoError(t, store.CreateGrant(ctx, u[1].ID, u[2].ID))

	users, err := store.WhitelistedUsers(ctx, u[0].ID)
	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.ElementsMatch(t, []string{u[1].ID, u[2].ID}, []string{users[0].ID, users[1].ID})

	// A used grant is no longer open.
	grant, err := store.LockOpenGrant(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	c := f.connect(t, 0, 1)
	require.NoError(t, store.UseGrant(ctx, grant, c.ID))

	users, err = store.WhitelistedUsers(ctx, u[0].ID)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, u[2].ID, users[0].ID)

	ok, err := store.OpenGrantExists(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.GrantExists(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = store.LockOpenGrant(ctx, u[0].ID, u[1].ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Deleting withdraws only the open grant asked for.
	require.NoError(t, store.DeleteOpenGrant(ctx, u[0].ID, u[1].ID))
	ok, err = store.GrantExists(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, store.DeleteOpenGrant(ctx, u[0].ID, u[2].ID))
	ok, err = store.OpenGrantExists(ctx, u[0].ID, u[2].ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.OpenGrantExists(ctx, u[1].ID, u[2].ID)
	require.NoError(t, err)
	assert.True(t, ok)
}

// Users: 0 asks 1; 2 and 3 are common connections, 4 is not among userIDs.
func TestMediationRequests(t *testing.T) {
	t.Parallel()

	f := newGraphFixture(t, 5)
	ctx, store, u := f.ctx, f.store, f.users
	ids := []string{u[0].ID, u[1].ID, u[2].ID, u[3].ID}

	require.NoError(t, store.CreateMediationRequest(ctx, u[0].ID, u[1].ID, "hi"))
	require.NoError(t, store.CreateMediationRequest(ctx, u[4].ID, u[1].ID, ""))

	req, err := store.MediationRequestBetween(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	require.NotNil(t, req.SourceNote)
	assert.Equal(t, "hi", *req.SourceNote)

	_, err = store.MediationRequestBetween(ctx, u[1].ID, u[0].ID)
	require.ErrorIs(t, err, repo.ErrNotFound)

	got, err := store.UndecidedRequestBetween(ctx, req.ID, ids)
	require.NoError(t, err)
	assert.Equal(t, req.ID, got.ID)
	_, err = store.UndecidedRequestBetween(ctx, req.ID, ids[1:])
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Nobody signed yet: nothing to decide, both mediators see it.
	toDecide, err := store.RequestsToDecide(ctx, u[1].ID)
	require.NoError(t, err)
	assert.Empty(t, toDecide)

	toMediate, err := store.RequestsToMediate(ctx, ids, u[2].ID)
	require.NoError(t, err)
	require.Len(t, toMediate, 1)
	assert.Equal(t, u[0].ID, toMediate[0].WhoUser.ID)
	assert.Equal(t, u[1].ID, toMediate[0].TargetUser.ID)

	require.NoError(t, store.AddMediatorDecision(ctx, req.ID, u[2].ID, model.ConnectionMediationDecisionSigned, ""))
	require.NoError(t, store.AddMediatorDecision(ctx, req.ID, u[3].ID, model.ConnectionMediationDecisionDismissed, "no"))

	toMediate, err = store.RequestsToMediate(ctx, ids, u[2].ID)
	require.NoError(t, err)
	assert.Empty(t, toMediate)

	toDecide, err = store.RequestsToDecide(ctx, u[1].ID)
	require.NoError(t, err)
	require.Len(t, toDecide, 1)
	assert.Equal(t, u[0].ID, toDecide[0].WhoUser.ID)
	require.Len(t, toDecide[0].MediationUserConnectionMediators, 1)
	assert.Equal(t, u[2].ID, toDecide[0].MediationUserConnectionMediators[0].User.ID)

	// The target decides; the request leaves both lists.
	_, err = store.LockUndecidedRequest(ctx, req.ID, u[0].ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	locked, err := store.LockUndecidedRequest(ctx, req.ID, u[1].ID)
	require.NoError(t, err)
	require.NoError(t, store.RecordTargetDecision(ctx, locked, model.ConnectionRequestDecisionDismissed, "", "later"))
	assert.Nil(t, locked.ConnectionID)

	_, err = store.LockUndecidedRequest(ctx, req.ID, u[1].ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	toDecide, err = store.RequestsToDecide(ctx, u[1].ID)
	require.NoError(t, err)
	assert.Empty(t, toDecide)

	decided, err := store.MediationRequestBetween(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	require.NotNil(t, decided.TargetDecision)
	assert.Equal(t, model.ConnectionRequestDecisionDismissed, *decided.TargetDecision)
	require.NotNil(t, decided.TargetNote)
	assert.Equal(t, "later", *decided.TargetNote)

	// Deleting removes the request with its mediators, inside a transaction.
	require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error {
		return tx.DeleteMediationRequestBetween(ctx, u[0].ID, u[1].ID)
	}))
	ok, err := store.MediationRequestExists(ctx, u[0].ID, u[1].ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = store.MediationRequestExists(ctx, u[4].ID, u[1].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	err = store.DeleteMediationRequestBetween(ctx, u[0].ID, u[1].ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}
