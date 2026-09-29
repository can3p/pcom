package userops_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// connect wraps factory.Connect, whose three return values (both directions
// of the edge, plus an error) don't fit testutil.Must's (value, error) shape.
func connect(t *testing.T, ctx context.Context, db *sqlx.DB, aID, bID string) {
	t.Helper()
	_, _, err := factory.Connect(ctx, db, aID, bID)
	require.NoError(t, err)
}

// secondDegreeTrio builds source -- mediator -- target, both edges direct,
// with no direct edge between source and target: source and target are
// second-degree connections of one another, through mediator.
func secondDegreeTrio(t *testing.T, ctx context.Context, db *sqlx.DB) (source, mediator, target *core.User) {
	t.Helper()

	source = testutil.Must(factory.User(ctx, db))(t)
	mediator = testutil.Must(factory.User(ctx, db))(t)
	target = testutil.Must(factory.User(ctx, db))(t)

	connect(t, ctx, db, source.ID, mediator.ID)
	connect(t, ctx, db, mediator.ID, target.ID)

	return source, mediator, target
}

func TestCreateConnection_Symmetry(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	alice := testutil.Must(factory.User(ctx, db))(t)
	bob := testutil.Must(factory.User(ctx, db))(t)

	conn1, conn2, err := userops.CreateConnection(ctx, db, alice.ID, bob.ID)
	require.NoError(t, err)

	require.Equal(t, alice.ID, conn1.User1ID)
	require.Equal(t, bob.ID, conn1.User2ID)
	require.Equal(t, bob.ID, conn2.User1ID)
	require.Equal(t, alice.ID, conn2.User2ID)
	require.NotEqual(t, conn1.ID, conn2.ID)

	// the connection is visible from both directions
	require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, alice.ID, bob.ID))(t))
	require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, bob.ID, alice.ID))(t))
}

func TestGetDirectAndSecondDegreeUserIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	// Triangle: a, b and c are all directly connected to one another, so
	// none of them has any second-degree connection left over.
	a, b, c := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
	connect(t, ctx, db, a.ID, b.ID)
	connect(t, ctx, db, b.ID, c.ID)
	connect(t, ctx, db, c.ID, a.ID)

	// Chain: d -- e -- f, no direct edge between d and f.
	d, e, f := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
	connect(t, ctx, db, d.ID, e.ID)
	connect(t, ctx, db, e.ID, f.ID)

	// Isolated user, no edges at all.
	g := testutil.Must(factory.User(ctx, db))(t)

	// Common friends: h is directly connected to i and j, who are both
	// directly connected to k, so k is a second-degree connection of h
	// reachable through two different mediators.
	h, i, j, k := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
	connect(t, ctx, db, h.ID, i.ID)
	connect(t, ctx, db, h.ID, j.ID)
	connect(t, ctx, db, i.ID, k.ID)
	connect(t, ctx, db, j.ID, k.ID)

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

	a, b, c, unrelated := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

	// a -- b -- c, so a and c are second degree connections.
	connect(t, ctx, db, a.ID, b.ID)
	connect(t, ctx, db, b.ID, c.ID)

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

	db := testdb.New(t).DB
	ctx := context.Background()

	cases := []struct {
		name       string
		whitelist  bool
		wantErr    bool
		wantExists bool
	}{
		{name: "a whitelisted target lets the source connect", whitelist: true, wantExists: true},
		{name: "without a whitelist grant the connection is refused", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

			if tc.whitelist {
				testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
			}

			err := userops.EstablishConnection(ctx, db, source.ID, target.ID)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.wantExists, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
		})
	}

	t.Run("establishing the connection revokes a pending mediation request between the same two users", func(t *testing.T) {
		t.Parallel()

		source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID, factory.WithSourceNote("please")))(t)
		testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)

		require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))
		require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
		require.Nil(t, testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t))
	})
}

func TestDropConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("no connection between the users is a no-op", func(t *testing.T) {
		t.Parallel()

		a, b := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		require.NoError(t, userops.DropConnection(ctx, db, a.ID, b.ID))
	})

	t.Run("cleans up the connection and the whitelist grant that created it", func(t *testing.T) {
		t.Parallel()

		source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
		require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))

		// whitelisted_connections.connection_id references user_connections(id),
		// so if the whitelist row that was consumed by EstablishConnection were
		// left behind, deleting the connection below would fail with a foreign
		// key violation instead of succeeding.
		require.NoError(t, userops.DropConnection(ctx, db, source.ID, target.ID))
		require.False(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
	})

	t.Run("cleans up the connection, the mediation request and its mediators", func(t *testing.T) {
		t.Parallel()

		source, target, mediator := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)
		testutil.Must(factory.MediatorDecision(ctx, db, req.ID, mediator.ID, core.ConnectionMediationDecisionSigned))(t)

		require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))
		require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))

		// user_connection_mediators.mediation_id and the mediation request's
		// connection_id both carry foreign keys, so DropConnection would fail
		// instead of succeeding if it didn't delete the mediator row and the
		// request before the connection itself.
		require.NoError(t, userops.DropConnection(ctx, db, source.ID, target.ID))
		require.False(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
		require.Nil(t, testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t))
	})
}

func TestRequestMediation(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("succeeds between second degree connections", func(t *testing.T) {
		t.Parallel()

		source, _, target := secondDegreeTrio(t, ctx, db)

		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, "please introduce us"))

		req := testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t)
		require.NotNil(t, req)
		require.Equal(t, "please introduce us", req.SourceNote.String)
	})

	guardCases := []struct {
		name          string
		setup         func(t *testing.T) (source, target string)
		wantErrSubstr string
	}{
		{
			name: "refuses a direct connection",
			setup: func(t *testing.T) (string, string) {
				a, b := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
				connect(t, ctx, db, a.ID, b.ID)
				return a.ID, b.ID
			},
		},
		{
			name: "refuses two unrelated users",
			setup: func(t *testing.T) (string, string) {
				a, b := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
				return a.ID, b.ID
			},
		},
		{
			name: "refuses a request that already exists",
			setup: func(t *testing.T) (string, string) {
				source, _, target := secondDegreeTrio(t, ctx, db)
				require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))
				return source.ID, target.ID
			},
			wantErrSubstr: "already been requested",
		},
		{
			name: "refuses a request toward an already-whitelisted target",
			setup: func(t *testing.T) (string, string) {
				source, _, target := secondDegreeTrio(t, ctx, db)
				testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
				return source.ID, target.ID
			},
			wantErrSubstr: "whitelisted connection",
		},
	}

	for _, tc := range guardCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, target := tc.setup(t)
			err := userops.RequestMediation(ctx, db, source, target, "")
			if tc.wantErrSubstr != "" {
				require.ErrorContains(t, err, tc.wantErrSubstr)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestRevokeMediationRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("removes the request and any mediator decisions on it", func(t *testing.T) {
		t.Parallel()

		who, target, mediator := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, who.ID, target.ID))(t)
		testutil.Must(factory.MediatorDecision(ctx, db, req.ID, mediator.ID, core.ConnectionMediationDecisionSigned))(t)

		// user_connection_mediators.mediation_id references the request, so
		// revoking would fail with a foreign key violation instead of
		// succeeding if the mediator row were not deleted first.
		require.NoError(t, userops.RevokeMediationRequest(ctx, db, who.ID, target.ID))
		require.Nil(t, testutil.Must(userops.GetMediationRequest(ctx, db, who.ID, target.ID))(t))
	})

	t.Run("errors when there is no request to revoke", func(t *testing.T) {
		t.Parallel()

		who, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		err := userops.RevokeMediationRequest(ctx, db, who.ID, target.ID)
		require.ErrorIs(t, err, userops.ErrNoConnectionRequest)
	})
}

func TestDecideForwardMediationRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("a common direct connection may sign the request", func(t *testing.T) {
		t.Parallel()

		source, mediator, target := secondDegreeTrio(t, ctx, db)
		require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

		req := testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t)

		require.NoError(t, userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "vouching"))

		// the (mediation_id, user_id) pair is unique, so signing again for the
		// same mediator would fail if the first decision were not recorded.
		err := userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "vouching")
		require.Error(t, err)
	})

	guardCases := []struct {
		name  string
		actor func(t *testing.T, source *core.User) string
	}{
		{
			name: "a user with no connection to either side cannot decide",
			actor: func(t *testing.T, source *core.User) string {
				return testutil.Must(factory.User(ctx, db))(t).ID
			},
		},
		{
			name: "a connection to only one side of the request cannot decide",
			actor: func(t *testing.T, source *core.User) string {
				halfConnected := testutil.Must(factory.User(ctx, db))(t)
				connect(t, ctx, db, source.ID, halfConnected.ID)
				return halfConnected.ID
			},
		},
	}

	for _, tc := range guardCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, _, target := secondDegreeTrio(t, ctx, db)
			require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))
			req := testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t)

			actorID := tc.actor(t, source)

			err := userops.DecideForwardMediationRequest(ctx, db, actorID, req.ID, core.ConnectionMediationDecisionSigned, "")
			require.ErrorContains(t, err, "No such request")
		})
	}
}

func TestDecideConnectionRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	outcomes := []struct {
		name          string
		decision      core.ConnectionRequestDecision
		note          string
		wantExists    bool
		wantConnValid bool
	}{
		{
			name:          "approving creates the connection and records the decision",
			decision:      core.ConnectionRequestDecisionApproved,
			note:          "welcome",
			wantExists:    true,
			wantConnValid: true,
		},
		{
			name:     "dismissing does not create a connection",
			decision: core.ConnectionRequestDecisionDismissed,
		},
	}

	for _, tc := range outcomes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
			req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

			require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, tc.decision, tc.note))
			require.Equal(t, tc.wantExists, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))

			got := testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t)
			require.NotNil(t, got)
			require.True(t, got.TargetDecision.Valid)
			require.Equal(t, tc.decision, got.TargetDecision.Val)
			require.Equal(t, tc.wantConnValid, got.ConnectionID.Valid)
			require.Equal(t, tc.note, got.TargetNote.String)
		})
	}

	t.Run("errors on an unknown request id", func(t *testing.T) {
		t.Parallel()

		target := testutil.Must(factory.User(ctx, db))(t)

		err := userops.DecideConnectionRequest(ctx, db, target.ID, "00000000-0000-0000-0000-000000000000", core.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})

	t.Run("a user who isn't the target cannot decide", func(t *testing.T) {
		t.Parallel()

		source, target, someoneElse := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

		err := userops.DecideConnectionRequest(ctx, db, someoneElse.ID, req.ID, core.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})
}

// https://github.com/can3p/pcom/issues/117
func TestDecideConnectionRequest_CannotBeDecidedTwice(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/117 - DecideConnectionRequest never checks TargetDecision IS NULL, so approve-then-reject (or the reverse) silently overwrites the first decision while the stale connection_id is left in place")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
	req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

	require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))

	// the request was already decided once: deciding it again must be refused.
	err := userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionDismissed, "")
	require.Error(t, err)
}

// https://github.com/can3p/pcom/issues/117
func TestDecideForwardMediationRequest_CannotSignAfterTargetDecided(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/117 - DecideForwardMediationRequest never checks whether the target already decided, so a mediator can still sign a closed request")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, mediator, target := secondDegreeTrio(t, ctx, db)
	require.NoError(t, userops.RequestMediation(ctx, db, source.ID, target.ID, ""))

	req := testutil.Must(userops.GetMediationRequest(ctx, db, source.ID, target.ID))(t)

	require.NoError(t, userops.DecideConnectionRequest(ctx, db, target.ID, req.ID, core.ConnectionRequestDecisionApproved, ""))

	// the target already decided: the mediator's signature must be refused.
	err := userops.DecideForwardMediationRequest(ctx, db, mediator.ID, req.ID, core.ConnectionMediationDecisionSigned, "")
	require.Error(t, err)
}

func TestIsConnectionAllowed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

	require.False(t, testutil.Must(userops.IsConnectionAllowed(ctx, db, source.ID, target.ID))(t))

	testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
	require.True(t, testutil.Must(userops.IsConnectionAllowed(ctx, db, source.ID, target.ID))(t))

	// once the grant is consumed by establishing the connection it no longer
	// counts as an outstanding allowance.
	require.NoError(t, userops.EstablishConnection(ctx, db, source.ID, target.ID))
	require.False(t, testutil.Must(userops.IsConnectionAllowed(ctx, db, source.ID, target.ID))(t))
}

func TestCreateConnection_UnknownUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	real := testutil.Must(factory.User(ctx, db))(t)

	// user2_id references users(id): connecting to a user that doesn't
	// exist must fail instead of silently inserting a dangling row.
	_, _, err := userops.CreateConnection(ctx, db, real.ID, "00000000-0000-0000-0000-000000000000")
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
	user := testutil.Must(factory.User(ctx, db))(t)

	cases := []struct {
		name string
		err  func() error
	}{
		{"GetConnectionRadius", func() error { _, err := userops.GetConnectionRadius(ctx, db, malformed, user.ID); return err }},
		{"RequestMediation", func() error { return userops.RequestMediation(ctx, db, malformed, user.ID, "") }},
		{"RevokeMediationRequest", func() error { return userops.RevokeMediationRequest(ctx, db, malformed, user.ID) }},
		{"DecideForwardMediationRequest", func() error {
			return userops.DecideForwardMediationRequest(ctx, db, malformed, "00000000-0000-0000-0000-000000000000", core.ConnectionMediationDecisionSigned, "")
		}},
		{"DecideConnectionRequest", func() error {
			return userops.DecideConnectionRequest(ctx, db, user.ID, malformed, core.ConnectionRequestDecisionApproved, "")
		}},
		{"EstablishConnection", func() error { return userops.EstablishConnection(ctx, db, malformed, user.ID) }},
		{"DropConnection", func() error { return userops.DropConnection(ctx, db, malformed, user.ID) }},
		{"GetMediationRequest", func() error { _, err := userops.GetMediationRequest(ctx, db, malformed, user.ID); return err }},
		{"IsConnectionAllowed", func() error { _, err := userops.IsConnectionAllowed(ctx, db, malformed, user.ID); return err }},
		{"GetDirectAndSecondDegreeUserIDs", func() error {
			_, _, _, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, malformed)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tc.err())
		})
	}
}

func TestDropConnectionGrant(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

	testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
	require.True(t, testutil.Must(userops.IsConnectionAllowed(ctx, db, source.ID, target.ID))(t))

	require.NoError(t, userops.DropConnectionGrant(ctx, db, target.ID, source.ID))
	require.False(t, testutil.Must(userops.IsConnectionAllowed(ctx, db, source.ID, target.ID))(t))
}
