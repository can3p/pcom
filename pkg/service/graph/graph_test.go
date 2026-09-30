package graph_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
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
		direct, second, via, err := graph.DirectAndSecondDegree(ctx, repo.Using(db), a.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{b.ID, c.ID}, direct)
		require.Empty(t, second)
		require.Empty(t, via)
	})

	t.Run("chain: second degree reaches one hop further", func(t *testing.T) {
		direct, second, via, err := graph.DirectAndSecondDegree(ctx, repo.Using(db), d.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{e.ID}, direct)
		require.Equal(t, []string{f.ID}, second)
		require.Equal(t, map[string][]string{f.ID: {e.ID}}, via)
	})

	t.Run("isolated user has neither direct nor second degree connections", func(t *testing.T) {
		direct, second, via, err := graph.DirectAndSecondDegree(ctx, repo.Using(db), g.ID)
		require.NoError(t, err)
		require.Empty(t, direct)
		require.Empty(t, second)
		require.Empty(t, via)
	})

	t.Run("common friend is reported once per mediator", func(t *testing.T) {
		direct, second, via, err := graph.DirectAndSecondDegree(ctx, repo.Using(db), h.ID)
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
		want    graph.Radius
		wantErr error
	}{
		{name: "same user", from: a.ID, to: a.ID, want: graph.RadiusSameUser},
		{name: "direct connection", from: a.ID, to: b.ID, want: graph.RadiusDirect},
		{name: "second degree connection", from: a.ID, to: c.ID, want: graph.RadiusSecondDegree},
		{name: "unrelated user", from: a.ID, to: unrelated.ID, want: graph.RadiusUnrelated},
		{name: "empty from id", from: "", to: a.ID, want: graph.RadiusUnknown, wantErr: graph.ErrUserNotSignedIn},
		{name: "empty to id", from: a.ID, to: "", want: graph.RadiusUnknown, wantErr: graph.ErrUserNotSignedIn},
		{name: "both ids empty", from: "", to: "", want: graph.RadiusUnknown, wantErr: graph.ErrUserNotSignedIn},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := graph.RadiusBetween(ctx, repo.Using(db), tc.from, tc.to)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// A malformed id isn't a "not found" case: the database rejects it outright
// and the graph wrappers must surface that error.
func TestMalformedUserID_GraphWrappersSurfaceAsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	const malformed = "not-a-valid-uuid"
	user := testutil.Must(factory.User(ctx, db))(t)

	cases := []struct {
		name string
		err  func() error
	}{
		{"GetConnectionRadius", func() error { _, err := graph.RadiusBetween(ctx, repo.Using(db), malformed, user.ID); return err }},
		{"GetDirectAndSecondDegreeUserIDs", func() error {
			_, _, _, err := graph.DirectAndSecondDegree(ctx, repo.Using(db), malformed)
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
