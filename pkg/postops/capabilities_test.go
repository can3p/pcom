package postops_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// TestCanSeePost covers CanSeePost over the full radius x visibility matrix,
// for a published post and for a draft.
func TestCanSeePost(t *testing.T) {
	t.Parallel()

	radii := []graph.Radius{
		graph.RadiusSameUser,
		graph.RadiusDirect,
		graph.RadiusSecondDegree,
		graph.RadiusUnrelated,
		graph.RadiusUnknown,
	}

	visibilities := []core.PostVisibility{
		core.PostVisibilityDirectOnly,
		core.PostVisibilitySecondDegree,
		core.PostVisibilityPublic,
	}

	// want[radius][visibility]
	want := map[graph.Radius]map[core.PostVisibility]bool{
		graph.RadiusSameUser: {
			core.PostVisibilityDirectOnly:   true,
			core.PostVisibilitySecondDegree: true,
			core.PostVisibilityPublic:       true,
		},
		graph.RadiusDirect: {
			core.PostVisibilityDirectOnly:   true,
			core.PostVisibilitySecondDegree: true,
			core.PostVisibilityPublic:       true,
		},
		graph.RadiusSecondDegree: {
			core.PostVisibilityDirectOnly:   false,
			core.PostVisibilitySecondDegree: true,
			core.PostVisibilityPublic:       true,
		},
		graph.RadiusUnrelated: {
			core.PostVisibilityDirectOnly:   false,
			core.PostVisibilitySecondDegree: false,
			core.PostVisibilityPublic:       true,
		},
		graph.RadiusUnknown: {
			core.PostVisibilityDirectOnly:   false,
			core.PostVisibilitySecondDegree: false,
			core.PostVisibilityPublic:       true,
		},
	}

	for _, radius := range radii {
		for _, vis := range visibilities {
			radius, vis := radius, vis
			t.Run(fmt.Sprintf("radius=%d/visibility=%s", radius, vis), func(t *testing.T) {
				t.Parallel()

				post := &core.Post{VisibilityRadius: vis, PublishedAt: null.TimeFrom(time.Now())}
				require.Equal(t, want[radius][vis], postops.CanSeePost(post, radius))

				// a draft is the author's only, whatever its visibility
				draft := &core.Post{VisibilityRadius: vis}
				require.Equal(t, radius == graph.RadiusSameUser, postops.CanSeePost(draft, radius))
			})
		}
	}
}

// TestGetPostCapabilities covers GetPostCapabilities over every radius; the
// function does not depend on the post's visibility at all.
func TestGetPostCapabilities(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		radius graph.Radius
		want   *postops.PostCapabilities
	}{
		{
			radius: graph.RadiusSameUser,
			want: &postops.PostCapabilities{
				CanViewComments:  true,
				CanLeaveComments: true,
				CanEdit:          true,
				CanShare:         true,
			},
		},
		{
			radius: graph.RadiusDirect,
			want: &postops.PostCapabilities{
				CanViewComments:  true,
				CanLeaveComments: true,
				CanEdit:          false,
				CanShare:         false,
			},
		},
		{
			radius: graph.RadiusSecondDegree,
			want:   &postops.PostCapabilities{},
		},
		{
			radius: graph.RadiusUnrelated,
			want:   &postops.PostCapabilities{},
		},
		{
			radius: graph.RadiusUnknown,
			want:   &postops.PostCapabilities{},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("radius=%d", tc.radius), func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, postops.GetPostCapabilities(tc.radius))
		})
	}
}
