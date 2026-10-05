package connections_test

import (
	"context"
	"sync"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// as is the acting user of a service call, known by id only.
func as(id string) *model.User {
	return &model.User{ID: id}
}

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
func secondDegreeTrio(t *testing.T, ctx context.Context, db *sqlx.DB) (source, mediator, target *model.User) {
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

	conn1, conn2, err := repo.Using(db).CreateConnection(ctx, alice.ID, bob.ID)
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

func TestEstablishConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
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

			err := svc.Connect(ctx, as(source.ID), target.ID)
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

		require.NoError(t, svc.Connect(ctx, as(source.ID), target.ID))
		require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
		require.Nil(t, testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t))
	})
}

func TestDropConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	t.Run("no connection between the users is a no-op", func(t *testing.T) {
		t.Parallel()

		a, b := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		require.NoError(t, svc.Drop(ctx, as(a.ID), b.ID))
	})

	t.Run("cleans up the connection and the whitelist grant that created it", func(t *testing.T) {
		t.Parallel()

		source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
		require.NoError(t, svc.Connect(ctx, as(source.ID), target.ID))

		// whitelisted_connections.connection_id references user_connections(id),
		// so if the whitelist row that was consumed by EstablishConnection were
		// left behind, deleting the connection below would fail with a foreign
		// key violation instead of succeeding.
		require.NoError(t, svc.Drop(ctx, as(source.ID), target.ID))
		require.False(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
	})

	t.Run("cleans up the connection, the mediation request and its mediators", func(t *testing.T) {
		t.Parallel()

		source, target, mediator := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)
		testutil.Must(factory.MediatorDecision(ctx, db, req.ID, mediator.ID, model.ConnectionMediationDecisionSigned))(t)

		require.NoError(t, svc.DecideRequest(ctx, as(target.ID), req.ID, model.ConnectionRequestDecisionApproved, ""))
		require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))

		// user_connection_mediators.mediation_id and the mediation request's
		// connection_id both carry foreign keys, so DropConnection would fail
		// instead of succeeding if it didn't delete the mediator row and the
		// request before the connection itself.
		require.NoError(t, svc.Drop(ctx, as(source.ID), target.ID))
		require.False(t, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))
		require.Nil(t, testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t))
	})
}

func TestRequestMediation(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	t.Run("succeeds between second degree connections", func(t *testing.T) {
		t.Parallel()

		source, _, target := secondDegreeTrio(t, ctx, db)

		require.NoError(t, svc.RequestMediation(ctx, as(source.ID), target.ID, "please introduce us"))

		req := testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t)
		require.NotNil(t, req)
		require.Equal(t, "please introduce us", lo.FromPtr(req.SourceNote))
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
				require.NoError(t, svc.RequestMediation(ctx, as(source.ID), target.ID, ""))
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
			err := svc.RequestMediation(ctx, as(source), target, "")
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
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	t.Run("removes the request and any mediator decisions on it", func(t *testing.T) {
		t.Parallel()

		who, target, mediator := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, who.ID, target.ID))(t)
		testutil.Must(factory.MediatorDecision(ctx, db, req.ID, mediator.ID, model.ConnectionMediationDecisionSigned))(t)

		// user_connection_mediators.mediation_id references the request, so
		// revoking would fail with a foreign key violation instead of
		// succeeding if the mediator row were not deleted first.
		require.NoError(t, svc.RevokeMediation(ctx, as(who.ID), target.ID))
		require.Nil(t, testutil.Must(svc.MediationRequest(ctx, as(who.ID), target.ID))(t))
	})

	t.Run("errors when there is no request to revoke", func(t *testing.T) {
		t.Parallel()

		who, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		err := svc.RevokeMediation(ctx, as(who.ID), target.ID)
		require.ErrorIs(t, err, connections.ErrNoConnectionRequest)
	})
}

func TestDecideForwardMediationRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	t.Run("a common direct connection may sign the request", func(t *testing.T) {
		t.Parallel()

		source, mediator, target := secondDegreeTrio(t, ctx, db)
		require.NoError(t, svc.RequestMediation(ctx, as(source.ID), target.ID, ""))

		req := testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t)

		require.NoError(t, svc.DecideMediation(ctx, as(mediator.ID), req.ID, model.ConnectionMediationDecisionSigned, "vouching"))

		// the (mediation_id, user_id) pair is unique, so signing again for the
		// same mediator would fail if the first decision were not recorded.
		err := svc.DecideMediation(ctx, as(mediator.ID), req.ID, model.ConnectionMediationDecisionSigned, "vouching")
		require.Error(t, err)
	})

	guardCases := []struct {
		name  string
		actor func(t *testing.T, source *model.User) string
	}{
		{
			name: "a user with no connection to either side cannot decide",
			actor: func(t *testing.T, source *model.User) string {
				return testutil.Must(factory.User(ctx, db))(t).ID
			},
		},
		{
			name: "a connection to only one side of the request cannot decide",
			actor: func(t *testing.T, source *model.User) string {
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
			require.NoError(t, svc.RequestMediation(ctx, as(source.ID), target.ID, ""))
			req := testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t)

			actorID := tc.actor(t, source)

			err := svc.DecideMediation(ctx, as(actorID), req.ID, model.ConnectionMediationDecisionSigned, "")
			require.ErrorContains(t, err, "No such request")
		})
	}
}

func TestDecideConnectionRequest(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	outcomes := []struct {
		name          string
		decision      model.ConnectionRequestDecision
		note          string
		wantExists    bool
		wantConnValid bool
	}{
		{
			name:          "approving creates the connection and records the decision",
			decision:      model.ConnectionRequestDecisionApproved,
			note:          "welcome",
			wantExists:    true,
			wantConnValid: true,
		},
		{
			name:     "dismissing does not create a connection",
			decision: model.ConnectionRequestDecisionDismissed,
		},
	}

	for _, tc := range outcomes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
			req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

			require.NoError(t, svc.DecideRequest(ctx, as(target.ID), req.ID, tc.decision, tc.note))
			require.Equal(t, tc.wantExists, testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t))

			got := testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t)
			require.NotNil(t, got)
			require.NotNil(t, got.TargetDecision)
			require.Equal(t, tc.decision, *got.TargetDecision)
			require.Equal(t, tc.wantConnValid, got.ConnectionID != nil)
			require.Equal(t, tc.note, lo.FromPtr(got.TargetNote))
		})
	}

	t.Run("errors on an unknown request id", func(t *testing.T) {
		t.Parallel()

		target := testutil.Must(factory.User(ctx, db))(t)

		err := svc.DecideRequest(ctx, as(target.ID), "00000000-0000-0000-0000-000000000000", model.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})

	t.Run("a user who isn't the target cannot decide", func(t *testing.T) {
		t.Parallel()

		source, target, someoneElse := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

		req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

		err := svc.DecideRequest(ctx, as(someoneElse.ID), req.ID, model.ConnectionRequestDecisionApproved, "")
		require.ErrorContains(t, err, "No such request")
	})
}

// https://github.com/can3p/pcom/issues/117
func TestDecideRequest_CannotBeDecidedTwice(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	approve := model.ConnectionRequestDecisionApproved
	dismiss := model.ConnectionRequestDecisionDismissed

	cases := []struct {
		name  string
		first model.ConnectionRequestDecision
		// second decision by the target, or a mediator's signature when nil
		second *model.ConnectionRequestDecision
	}{
		{"approve then dismiss", approve, &dismiss},
		{"dismiss then approve", dismiss, &approve},
		{"approve then approve", approve, &approve},
		{"approve then mediator signs", approve, nil},
		{"dismiss then mediator signs", dismiss, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, mediator, target := secondDegreeTrio(t, ctx, db)
			require.NoError(t, svc.RequestMediation(ctx, as(source.ID), target.ID, ""))
			req := testutil.Must(svc.MediationRequest(ctx, as(source.ID), target.ID))(t)

			require.NoError(t, svc.DecideRequest(ctx, as(target.ID), req.ID, tc.first, ""))
			before := testutil.Must(factory.GetMediationRequest(ctx, db, req.ID))(t)

			var err error
			if tc.second != nil {
				err = svc.DecideRequest(ctx, as(target.ID), req.ID, *tc.second, "")
			} else {
				err = svc.DecideMediation(ctx, as(mediator.ID), req.ID, model.ConnectionMediationDecisionSigned, "")
			}
			require.ErrorContains(t, err, "No such request")

			after := testutil.Must(factory.GetMediationRequest(ctx, db, req.ID))(t)
			require.Equal(t, tc.first, *after.TargetDecision)
			require.Equal(t, before.TargetDecidedAt, after.TargetDecidedAt)
			require.Equal(t, before.ConnectionID, after.ConnectionID)
			require.Equal(t, tc.first == approve, after.ConnectionID != nil)

			connected := testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t)
			require.Equal(t, tc.first == approve, connected)
			require.Empty(t, testutil.Must(factory.ListMediatorDecisions(ctx, db, req.ID))(t))
		})
	}
}

// https://github.com/can3p/pcom/issues/117
func TestDecideConnectionRequest_ConcurrentDecisionsAreSerialized(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	// several rounds make the race likely to show up without the guard
	for range 10 {
		source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)
		req := testutil.Must(factory.MediationRequest(ctx, db, source.ID, target.ID))(t)

		const runners = 2
		start := make(chan struct{})
		errs := make(chan error, runners)

		var wg sync.WaitGroup
		// approve and dismiss race: the duplicate-connection unique index would
		// mask two approvals, but two different decisions must not both win.
		for _, decision := range []model.ConnectionRequestDecision{model.ConnectionRequestDecisionApproved, model.ConnectionRequestDecisionDismissed} {
			wg.Go(func() {
				<-start
				errs <- svc.DecideRequest(ctx, as(target.ID), req.ID, decision, "")
			})
		}

		close(start)
		wg.Wait()
		close(errs)

		var failures int
		for err := range errs {
			if err != nil {
				failures++
			}
		}
		require.Equal(t, 1, failures, "exactly one of two racing decisions must be refused")

		got := testutil.Must(factory.GetMediationRequest(ctx, db, req.ID))(t)
		approved := *got.TargetDecision == model.ConnectionRequestDecisionApproved
		require.Equal(t, approved, got.ConnectionID != nil)

		connected := testutil.Must(factory.ConnectionExists(ctx, db, source.ID, target.ID))(t)
		require.Equal(t, approved, connected)
	}
}

func TestIsConnectionAllowed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

	require.False(t, testutil.Must(svc.IsConnectionAllowed(ctx, as(source.ID), target.ID))(t))

	testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
	require.True(t, testutil.Must(svc.IsConnectionAllowed(ctx, as(source.ID), target.ID))(t))

	// once the grant is consumed by establishing the connection it no longer
	// counts as an outstanding allowance.
	require.NoError(t, svc.Connect(ctx, as(source.ID), target.ID))
	require.False(t, testutil.Must(svc.IsConnectionAllowed(ctx, as(source.ID), target.ID))(t))
}

func TestCreateConnection_UnknownUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	real := testutil.Must(factory.User(ctx, db))(t)

	// user2_id references users(id): connecting to a user that doesn't
	// exist must fail instead of silently inserting a dangling row.
	_, _, err := repo.Using(db).CreateConnection(ctx, real.ID, "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
}

// A malformed id isn't a "not found" case (sql.ErrNoRows): the database
// rejects it outright, and every function here must surface that error
// instead of mistaking it for "no such request/connection".
func TestMalformedUserID_SurfacesAsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	const malformed = "not-a-valid-uuid"
	user := testutil.Must(factory.User(ctx, db))(t)

	cases := []struct {
		name string
		err  func() error
	}{
		{"RequestMediation", func() error { return svc.RequestMediation(ctx, as(malformed), user.ID, "") }},
		{"RevokeMediationRequest", func() error { return svc.RevokeMediation(ctx, as(malformed), user.ID) }},
		{"DecideForwardMediationRequest", func() error {
			return svc.DecideMediation(ctx, as(malformed), "00000000-0000-0000-0000-000000000000", model.ConnectionMediationDecisionSigned, "")
		}},
		{"DecideConnectionRequest", func() error {
			return svc.DecideRequest(ctx, as(user.ID), malformed, model.ConnectionRequestDecisionApproved, "")
		}},
		{"EstablishConnection", func() error { return svc.Connect(ctx, as(malformed), user.ID) }},
		{"DropConnection", func() error { return svc.Drop(ctx, as(malformed), user.ID) }},
		{"GetMediationRequest", func() error { _, err := svc.MediationRequest(ctx, as(malformed), user.ID); return err }},
		{"IsConnectionAllowed", func() error { _, err := svc.IsConnectionAllowed(ctx, as(malformed), user.ID); return err }},
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
	svc := connections.New(repo.New(db))
	ctx := context.Background()

	source, target := testutil.Must(factory.User(ctx, db))(t), testutil.Must(factory.User(ctx, db))(t)

	testutil.Must(factory.Whitelist(ctx, db, target.ID, source.ID))(t)
	require.True(t, testutil.Must(svc.IsConnectionAllowed(ctx, as(source.ID), target.ID))(t))

	require.NoError(t, svc.RemoveFromWhitelist(ctx, as(target.ID), source.ID))
	require.False(t, testutil.Must(svc.IsConnectionAllowed(ctx, as(source.ID), target.ID))(t))
}
