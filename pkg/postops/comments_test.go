package postops_test

import (
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/stretchr/testify/require"
)

// mkComment builds an in-memory *model.PostComment, the way a pure unit test
// (no database) has to: comments_test.go never touches the ORM, so
// pkg/testutil/factory (which inserts) does not apply here.
func mkComment(id, parentID string, createdAt time.Time, username string) *model.PostComment {
	c := &model.PostComment{
		ID:        id,
		CreatedAt: createdAt,
	}

	if parentID != "" {
		c.ParentCommentID = new(parentID)
	}

	c.User = &model.User{Username: username}

	return c
}

func base(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
}

// idLevels flattens ConstructComments's output into (id, level) pairs so the
// table below can assert order and nesting in one shot.
func idLevels(comments []*postops.Comment) []struct {
	ID    string
	Level int64
} {
	out := make([]struct {
		ID    string
		Level int64
	}, len(comments))

	for i, c := range comments {
		out[i] = struct {
			ID    string
			Level int64
		}{ID: c.ID, Level: c.Level}
	}

	return out
}

func TestConstructComments(t *testing.T) {
	t.Parallel()

	t0 := base(t)

	testCases := []struct {
		name     string
		comments []*model.PostComment
		want     []struct {
			ID    string
			Level int64
		}
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     nil,
		},
		{
			name: "flat list is ordered by creation time regardless of input order",
			comments: []*model.PostComment{
				mkComment("c3", "", t0.Add(3*time.Minute), "carol"),
				mkComment("c1", "", t0.Add(1*time.Minute), "alice"),
				mkComment("c2", "", t0.Add(2*time.Minute), "bob"),
			},
			want: []struct {
				ID    string
				Level int64
			}{
				{"c1", 0}, {"c2", 0}, {"c3", 0},
			},
		},
		{
			name: "a reply chain is nested depth-first with increasing levels",
			comments: []*model.PostComment{
				mkComment("c1", "", t0, "alice"),
				mkComment("c2", "c1", t0.Add(1*time.Minute), "bob"),
				mkComment("c3", "c2", t0.Add(2*time.Minute), "carol"),
			},
			want: []struct {
				ID    string
				Level int64
			}{
				{"c1", 0}, {"c2", 1}, {"c3", 2},
			},
		},
		{
			name: "siblings under the same parent keep their own creation order",
			comments: []*model.PostComment{
				mkComment("p", "", t0, "alice"),
				mkComment("child2", "p", t0.Add(2*time.Minute), "carol"),
				mkComment("child1", "p", t0.Add(1*time.Minute), "bob"),
			},
			want: []struct {
				ID    string
				Level int64
			}{
				{"p", 0}, {"child1", 1}, {"child2", 1},
			},
		},
		{
			name: "a whole thread is fully visited before the next top-level thread starts",
			comments: []*model.PostComment{
				mkComment("a", "", t0, "alice"),
				mkComment("b", "", t0.Add(4*time.Minute), "dave"),
				mkComment("a1", "a", t0.Add(1*time.Minute), "bob"),
				mkComment("a2", "a", t0.Add(2*time.Minute), "carol"),
				mkComment("b1", "b", t0.Add(5*time.Minute), "erin"),
			},
			want: []struct {
				ID    string
				Level int64
			}{
				{"a", 0}, {"a1", 1}, {"a2", 1}, {"b", 0}, {"b1", 1},
			},
		},
		{
			// Characterization: a reply whose parent is not present among the
			// given comments (e.g. the parent was deleted) is silently
			// dropped from the flattened output, because it is only ever
			// reached by walking down from a comment that made it into the
			// result. This pins the current behavior; it is not necessarily
			// the intended one.
			name: "a reply to a missing parent is dropped, not shown top-level",
			comments: []*model.PostComment{
				mkComment("visible", "", t0, "alice"),
				mkComment("orphan", "does-not-exist", t0.Add(1*time.Minute), "bob"),
			},
			want: []struct {
				ID    string
				Level int64
			}{
				{"visible", 0},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := postops.ConstructComments(nil, tc.comments, graph.RadiusDirect)

			if len(tc.want) == 0 {
				require.Empty(t, got)
				return
			}

			require.Equal(t, tc.want, idLevels(got))
		})
	}
}

func TestConstructComments_Capabilities(t *testing.T) {
	t.Parallel()

	t0 := base(t)
	comments := []*model.PostComment{
		mkComment("c1", "", t0, "alice"),
	}

	sameUser := postops.ConstructComments(nil, comments, graph.RadiusSameUser)
	require.True(t, sameUser[0].Capabilities.CanRespond)

	direct := postops.ConstructComments(nil, comments, graph.RadiusDirect)
	require.True(t, direct[0].Capabilities.CanRespond)

	unrelated := postops.ConstructComments(nil, comments, graph.RadiusUnrelated)
	require.False(t, unrelated[0].Capabilities.CanRespond)
}

func TestConstructComments_CanEdit(t *testing.T) {
	t.Parallel()

	t0 := base(t)
	comment := mkComment("c1", "", t0, "alice")
	comment.UserID = "alice-id"
	comments := []*model.PostComment{comment}
	alice := &model.User{ID: "alice-id"}

	require.True(t, postops.ConstructComments(alice, comments, graph.RadiusDirect)[0].Capabilities.CanEdit)
	require.True(t, postops.ConstructComments(alice, comments, graph.RadiusSameUser)[0].Capabilities.CanEdit)
	// lost the connection to the post's author
	require.False(t, postops.ConstructComments(alice, comments, graph.RadiusUnrelated)[0].Capabilities.CanEdit)
	require.False(t, postops.ConstructComments(&model.User{ID: "bob-id"}, comments, graph.RadiusDirect)[0].Capabilities.CanEdit)
	require.False(t, postops.ConstructComments(nil, comments, graph.RadiusDirect)[0].Capabilities.CanEdit)
}

func TestConstructComments_AuthorIsCarriedThrough(t *testing.T) {
	t.Parallel()

	t0 := base(t)
	comments := []*model.PostComment{
		mkComment("c1", "", t0, "alice"),
	}

	got := postops.ConstructComments(nil, comments, graph.RadiusDirect)
	require.Equal(t, "alice", got[0].Author.Username)
}
