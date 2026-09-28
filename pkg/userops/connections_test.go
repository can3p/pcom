package userops_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// newSecondDegreeTrio builds source -- mediator -- target, both edges
// direct, with no direct edge between source and target: source and target
// are second-degree connections of one another, through mediator.
func newSecondDegreeTrio(t *testing.T, ctx context.Context, db *sqlx.DB) (source, mediator, target *core.User) {
	t.Helper()

	var err error
	source, err = factory.User(ctx, db)
	require.NoError(t, err)
	mediator, err = factory.User(ctx, db)
	require.NoError(t, err)
	target, err = factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, source.ID, mediator.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, mediator.ID, target.ID)
	require.NoError(t, err)

	return source, mediator, target
}

func TestCreateConnection_Symmetry(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	alice, err := factory.User(ctx, db)
	require.NoError(t, err)
	bob, err := factory.User(ctx, db)
	require.NoError(t, err)

	conn1, conn2, err := userops.CreateConnection(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)

	require.Equal(t, alice.ID, conn1.User1ID)
	require.Equal(t, bob.ID, conn1.User2ID)
	require.Equal(t, bob.ID, conn2.User1ID)
	require.Equal(t, alice.ID, conn2.User2ID)
	require.NotEqual(t, conn1.ID, conn2.ID)

	// the connection is visible from both directions
	exists, err := factory.ConnectionExists(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = factory.ConnectionExists(ctx, db, bob.ID, alice.ID)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestGetDirectAndSecondDegreeUserIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	// Triangle: a, b and c are all directly connected to one another, so
	// none of them has any second-degree connection left over.
	a, err := factory.User(ctx, db)
	require.NoError(t, err)
	b, err := factory.User(ctx, db)
	require.NoError(t, err)
	c, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, a.ID, b.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, b.ID, c.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, c.ID, a.ID)
	require.NoError(t, err)

	// Chain: d -- e -- f, no direct edge between d and f.
	d, err := factory.User(ctx, db)
	require.NoError(t, err)
	e, err := factory.User(ctx, db)
	require.NoError(t, err)
	f, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, d.ID, e.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, e.ID, f.ID)
	require.NoError(t, err)

	// Isolated user, no edges at all.
	g, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Common friends: h is directly connected to i and j, who are both
	// directly connected to k, so k is a second-degree connection of h
	// reachable through two different mediators.
	h, err := factory.User(ctx, db)
	require.NoError(t, err)
	i, err := factory.User(ctx, db)
	require.NoError(t, err)
	j, err := factory.User(ctx, db)
	require.NoError(t, err)
	k, err := factory.User(ctx, db)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, h.ID, i.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, h.ID, j.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, i.ID, k.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, j.ID, k.ID)
	require.NoError(t, err)

	t.Run("triangle: direct connections are excluded from second degree", func(t *testing.T) {
		direct, second, via, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, a.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{b.ID, c.ID}, direct)
		require.Empty(t, second)
		require.Empty(t, via)
	})

	t.Run("chain: second degree reaches one hop further", func(t *testing.T) {
		direct, second, via, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, d.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{e.ID}, direct)
		require.Equal(t, []string{f.ID}, second)
		require.Equal(t, map[string][]string{f.ID: {e.ID}}, via)
	})

	t.Run("isolated user has neither direct nor second degree connections", func(t *testing.T) {
		direct, second, via, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, g.ID)
		require.NoError(t, err)
		require.Empty(t, direct)
		require.Empty(t, second)
		require.Empty(t, via)
	})

	t.Run("common friend is reported once per mediator", func(t *testing.T) {
		direct, second, via, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, h.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{i.ID, j.ID}, direct)
		require.ElementsMatch(t, []string{k.ID}, lo.Uniq(second), "k is the only second-degree connection")
		// k is reachable via both i and j: multiplicity is only meaningful
		// through the via map, not through how many times k appears in second.
		require.ElementsMatch(t, []string{i.ID, j.ID}, via[k.ID])
	})
}

func TestGetConnectionRadius(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	a, err := factory.User(ctx, db)
	require.NoError(t, err)
	b, err := factory.User(ctx, db)
	require.NoError(t, err)
	c, err := factory.User(ctx, db)
	require.NoError(t, err)
	unrelated, err := factory.User(ctx, db)
	require.NoError(t, err)

	// a -- b -- c, so a and c are second degree connections.
	_, _, err = factory.Connect(ctx, db, a.ID, b.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, db, b.ID, c.ID)
	require.NoError(t, err)

	tests := []struct {
		name    string
		from    string
		to      string
		want    userops.ConnectionRadius
		wantErr error
	}{
		{name: "same user", from: a.ID, to: a.ID, want: userops.ConnectionRadiusSameUser},
		{name: "direct connection", from: a.ID, to: b.ID, want: userops.ConnectionRadiusDirect},
		{name: "second degree connection", from: a.ID, to: c.ID, want: userops.ConnectionRadiusSecondDegree},
		{name: "unrelated user", from: a.ID, to: unrelated.ID, want: userops.ConnectionRadiusUnrelated},
		{name: "empty from id", from: "", to: a.ID, want: userops.ConnectionRadiusUnknown, wantErr: userops.ErrUserNotSignedIn},
		{name: "empty to id", from: a.ID, to: "", want: userops.ConnectionRadiusUnknown, wantErr: userops.ErrUserNotSignedIn},
		{name: "both ids empty", from: "", to: "", want: userops.ConnectionRadiusUnknown, wantErr: userops.ErrUserNotSignedIn},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := userops.GetConnectionRadius(ctx, db, tc.from, tc.to)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEstablishConnection(t *testing.T) {
	t.Parallel()

	t.Run("a whitelisted target lets the source connect", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		_, err = factory.Whitelist(ctx, db, target.ID, source.ID)
		require.NoError(t, err)

		require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("without a whitelist grant the connection is refused", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		err = userops.EstablishConnection(ctx, db, source.ID, target.ID)
		require.Error(t, err)

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("establishing the connection revokes a pending mediation request between the same two users", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		_, err = factory.MediationRequest(ctx, db, source.ID, target.ID, factory.WithSourceNote("please"))
		require.NoError(t, err)

		_, err = factory.Whitelist(ctx, db, target.ID, source.ID)
		require.NoError(t, err)

		require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.True(t, exists)

		req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.Nil(t, req)
	})
}

func TestDropConnection(t *testing.T) {
	t.Parallel()

	t.Run("no connection between the users is a no-op", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		a, err := factory.User(ctx, db)
		require.NoError(t, err)
		b, err := factory.User(ctx, db)
		require.NoError(t, err)

		require.NoError(t, userops.DropConnection(ctx, db, a.ID, b.ID))
	})

	t.Run("cleans up the connection and the whitelist grant that created it", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		_, err = factory.Whitelist(ctx, db, target.ID, source.ID)
		require.NoError(t, err)
		require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))

		// whitelisted_connections.connection_id references user_connections(id),
		// so if the whitelist row that was consumed by EstablishConnection were
		// left behind, deleting the connection below would fail with a foreign
		// key violation instead of succeeding.
		require.NoError(t, userops.DropConnection(ctx, db, source.ID, target.ID))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("cleans up the connection, the mediation request and its mediators", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)
		mediator, err := factory.User(ctx, db)
		require.NoError(t, err)

		req, err := factory.MediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		_, err = factory.MediatorDecision(ctx, db, req.ID, mediator.ID, core.ConnectionMediationDecisionSigned)
		require.NoError(t, err)

		require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.True(t, exists)

		// user_connection_mediators.mediation_id and the mediation request's
		// connection_id both carry foreign keys, so DropConnection would fail
		// instead of succeeding if it didn't delete the mediator row and the
		// request before the connection itself.
		require.NoError(t, userops.DropConnection(ctx, db, source.ID, target.ID))

		exists, err = factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.False(t, exists)

		got, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRequestMediation(t *testing.T) {
	t.Parallel()

	t.Run("succeeds between second degree connections", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, _, target := newSecondDegreeTrio(t, ctx, db)

		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, "please introduce us"))

		req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.NotNil(t, req)
		require.Equal(t, "please introduce us", req.SourceNote.String)
	})

	t.Run("refuses a direct connection", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		a, err := factory.User(ctx, db)
		require.NoError(t, err)
		b, err := factory.User(ctx, db)
		require.NoError(t, err)
		_, _, err = factory.Connect(ctx, db, a.ID, b.ID)
		require.NoError(t, err)

		require.Error(t, userops.RequestMediation(ctx, db, a.ID, b.ID, ""))
	})

	t.Run("refuses two unrelated users", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		a, err := factory.User(ctx, db)
		require.NoError(t, err)
		b, err := factory.User(ctx, db)
		require.NoError(t, err)

		require.Error(t, userops.RequestMediation(ctx, db, a.ID, b.ID, ""))
	})

	t.Run("refuses a request that already exists", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, _, target := newSecondDegreeTrio(t, ctx, db)

		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

		err := userops.RequestMediation(ctx, db, source.ID, target.ID, "")
		require.ErrorContains(t, err, "already been requested")
	})

	t.Run("refuses a request toward an already-whitelisted target", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, _, target := newSecondDegreeTrio(t, ctx, db)

		_, err := factory.Whitelist(ctx, db, target.ID, source.ID)
		require.NoError(t, err)

		err = userops.RequestMediation(ctx, db, source.ID, target.ID, "")
		require.ErrorContains(t, err, "whitelisted connection")
	})
}

func TestRevokeMediationRequest(t *testing.T) {
	t.Parallel()

	t.Run("removes the request and any mediator decisions on it", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		who, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)
		mediator, err := factory.User(ctx, db)
		require.NoError(t, err)

		req, err := factory.MediationRequest(ctx, db, who.ID, target.ID)
		require.NoError(t, err)
		_, err = factory.MediatorDecision(ctx, db, req.ID, mediator.ID, core.ConnectionMediationDecisionSigned)
		require.NoError(t, err)

		// user_connection_mediators.mediation_id references the request, so
		// revoking would fail with a foreign key violation instead of
		// succeeding if the mediator row were not deleted first.
		require.NoError(t, userops.RevokeMediationRequest(ctx, db, who.ID, target.ID))

		got, err := userops.GetMediationRequest(ctx, db, who.ID, target.ID)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("errors when there is no request to revoke", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		who, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		err = userops.RevokeMediationRequest(ctx, db, who.ID, target.ID)
		require.ErrorIs(t, err, userops.ErrNoConnectionRequest)
	})
}

func TestDecideForwardMediationRequest(t *testing.T) {
	t.Parallel()

	t.Run("a common direct connection may sign the request", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, mediator, target := newSecondDegreeTrio(t, ctx, db)
		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

		req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		require.NoError(t, userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "vouching"))

		// the (mediation_id, user_id) pair is unique, so signing again for the
		// same mediator would fail if the first decision were not recorded.
		err = userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "vouching")
		require.Error(t, err)
	})

	t.Run("a user with no connection to either side cannot decide", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, _, target := newSecondDegreeTrio(t, ctx, db)
		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

		req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		stranger, err := factory.User(ctx, db)
		require.NoError(t, err)

		err = userops.DecideForwardMediationRequest(ctx, db, stranger.ID, req.ID, core.ConnectionMediationDecisionSigned, "")
		require.ErrorContains(t, err, "No such request")
	})

	t.Run("a connection to only one side of the request cannot decide", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, _, target := newSecondDegreeTrio(t, ctx, db)
		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

		req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		halfConnected, err := factory.User(ctx, db)
		require.NoError(t, err)
		_, _, err = factory.Connect(ctx, db, source.ID, halfConnected.ID)
		require.NoError(t, err)

		err = userops.DecideForwardMediationRequest(ctx, db, halfConnected.ID, req.ID, core.ConnectionMediationDecisionSigned, "")
		require.ErrorContains(t, err, "No such request")
	})
}

func TestDecideConnectionRequest(t *testing.T) {
	t.Parallel()

	t.Run("approving creates the connection and records the decision", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		req, err := factory.MediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, "welcome"))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.True(t, exists)

		got, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.True(t, got.TargetDecision.Valid)
		require.Equal(t, core.ConnectionRequestDecisionApproved, got.TargetDecision.Val)
		require.True(t, got.ConnectionID.Valid)
		require.Equal(t, "welcome", got.TargetNote.String)
	})

	t.Run("dismissing does not create a connection", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		req, err := factory.MediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionDismissed, ""))

		exists, err := factory.ConnectionExists(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.False(t, exists)

		got, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.True(t, got.TargetDecision.Valid)
		require.Equal(t, core.ConnectionRequestDecisionDismissed, got.TargetDecision.Val)
		require.False(t, got.ConnectionID.Valid)
	})

	t.Run("errors on an unknown request id", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		target, err := factory.User(ctx, db)
		require.NoError(t, err)

		err = userops.DecideConnectionRequest(ctx, db, target.ID, "00000000-0000-0000-0000-000000000000", core.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})

	t.Run("a user who isn't the target cannot decide", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		ctx := context.Background()

		source, err := factory.User(ctx, db)
		require.NoError(t, err)
		target, err := factory.User(ctx, db)
		require.NoError(t, err)
		someoneElse, err := factory.User(ctx, db)
		require.NoError(t, err)

		req, err := factory.MediationRequest(ctx, db, source.ID, target.ID)
		require.NoError(t, err)

		err = userops.DecideConnectionRequest(ctx, db, someoneElse.ID, req.ID, core.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})
}

// https://github.com/can3p/pcom/issues/117
func TestDecideConnectionRequest_CannotBeDecidedTwice(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/117 - DecideConnectionRequest never checks TargetDecision IS NULL, so approve-then-reject (or the reverse) silently overwrites the first decision while the stale connection_id is left in place")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, err := factory.User(ctx, db)
	require.NoError(t, err)
	target, err := factory.User(ctx, db)
	require.NoError(t, err)

	req, err := factory.MediationRequest(ctx, db, source.ID, target.ID)
	require.NoError(t, err)

	require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))

	// the request was already decided once: deciding it again must be refused.
	err = userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionDismissed, "")
	require.Error(t, err)
}

// https://github.com/can3p/pcom/issues/117
func TestDecideForwardMediationRequest_CannotSignAfterTargetDecided(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/117 - DecideForwardMediationRequest never checks whether the target already decided, so a mediator can still sign a closed request")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, mediator, target := newSecondDegreeTrio(t, ctx, db)
	require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

	req, err := userops.GetMediationRequest(ctx, db, source.ID, target.ID)
	require.NoError(t, err)

	require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))

	// the target already decided: the mediator's signature must be refused.
	err = userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "")
	require.Error(t, err)
}

func TestIsConnectionAllowed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, err := factory.User(ctx, db)
	require.NoError(t, err)
	target, err := factory.User(ctx, db)
	require.NoError(t, err)

	allowed, err := userops.IsConnectionAllowed(ctx, db, source.ID, target.ID)
	require.NoError(t, err)
	require.False(t, allowed)

	_, err = factory.Whitelist(ctx, db, target.ID, source.ID)
	require.NoError(t, err)

	allowed, err = userops.IsConnectionAllowed(ctx, db, source.ID, target.ID)
	require.NoError(t, err)
	require.True(t, allowed)

	// once the grant is consumed by establishing the connection it no longer
	// counts as an outstanding allowance.
	require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))

	allowed, err = userops.IsConnectionAllowed(ctx, db, source.ID, target.ID)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestCreateConnection_UnknownUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	real, err := factory.User(ctx, db)
	require.NoError(t, err)

	// user2_id references users(id): connecting to a user that doesn't
	// exist must fail instead of silently inserting a dangling row.
	_, _, err = userops.CreateConnection(ctx, db, real.ID, "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
}

// A malformed id isn't a "not found" case (sql.ErrNoRows): the database
// rejects it outright, and every function here must surface that error
// instead of mistaking it for "no such request/connection".
func TestMalformedUserID_SurfacesAsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	const malformed = "not-a-valid-uuid"

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	t.Run("GetConnectionRadius", func(t *testing.T) {
		t.Parallel()
		_, err := userops.GetConnectionRadius(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("RequestMediation", func(t *testing.T) {
		t.Parallel()
		err := userops.RequestMediation(ctx, db, malformed, user.ID, "")
		require.Error(t, err)
	})

	t.Run("RevokeMediationRequest", func(t *testing.T) {
		t.Parallel()
		err := userops.RevokeMediationRequest(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("DecideForwardMediationRequest", func(t *testing.T) {
		t.Parallel()
		err := userops.DecideForwardMediationRequest(ctx, db, malformed, "00000000-0000-0000-0000-000000000000", core.ConnectionMediationDecisionSigned, "")
		require.Error(t, err)
	})

	t.Run("DecideConnectionRequest", func(t *testing.T) {
		t.Parallel()
		err := userops.DecideConnectionRequest(ctx, db, user.ID, malformed, core.ConnectionRequestDecisionApproved, "")
		require.Error(t, err)
	})

	t.Run("EstablishConnection", func(t *testing.T) {
		t.Parallel()
		err := userops.EstablishConnection(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("DropConnection", func(t *testing.T) {
		t.Parallel()
		err := userops.DropConnection(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("GetMediationRequest", func(t *testing.T) {
		t.Parallel()
		_, err := userops.GetMediationRequest(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("IsConnectionAllowed", func(t *testing.T) {
		t.Parallel()
		_, err := userops.IsConnectionAllowed(ctx, db, malformed, user.ID)
		require.Error(t, err)
	})

	t.Run("GetDirectAndSecondDegreeUserIDs", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, malformed)
		require.Error(t, err)
	})
}

func TestDropConnectionGrant(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, err := factory.User(ctx, db)
	require.NoError(t, err)
	target, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Whitelist(ctx, db, target.ID, source.ID)
	require.NoError(t, err)

	allowed, err := userops.IsConnectionAllowed(ctx, db, source.ID, target.ID)
	require.NoError(t, err)
	require.True(t, allowed)

	require.NoError(t, userops.DropConnectionGrant(ctx, db, target.ID, source.ID))

	allowed, err = userops.IsConnectionAllowed(ctx, db, source.ID, target.ID)
	require.NoError(t, err)
	require.False(t, allowed)
}
