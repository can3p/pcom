package postops_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/stretchr/testify/require"
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

	visibilities := []model.PostVisibility{
		model.PostVisibilityDirectOnly,
		model.PostVisibilitySecondDegree,
		model.PostVisibilityPublic,
	}

	// want[radius][visibility]
	want := map[graph.Radius]map[model.PostVisibility]bool{
		graph.RadiusSameUser: {
			model.PostVisibilityDirectOnly:   true,
			model.PostVisibilitySecondDegree: true,
			model.PostVisibilityPublic:       true,
		},
		graph.RadiusDirect: {
			model.PostVisibilityDirectOnly:   true,
			model.PostVisibilitySecondDegree: true,
			model.PostVisibilityPublic:       true,
		},
		graph.RadiusSecondDegree: {
			model.PostVisibilityDirectOnly:   false,
			model.PostVisibilitySecondDegree: true,
			model.PostVisibilityPublic:       true,
		},
		graph.RadiusUnrelated: {
			model.PostVisibilityDirectOnly:   false,
			model.PostVisibilitySecondDegree: false,
			model.PostVisibilityPublic:       true,
		},
		graph.RadiusUnknown: {
			model.PostVisibilityDirectOnly:   false,
			model.PostVisibilitySecondDegree: false,
			model.PostVisibilityPublic:       true,
		},
	}

	for _, radius := range radii {
		for _, vis := range visibilities {
			radius, vis := radius, vis
			t.Run(fmt.Sprintf("radius=%d/visibility=%s", radius, vis), func(t *testing.T) {
				t.Parallel()

				post := &model.Post{VisibilityRadius: vis, PublishedAt: new(time.Now())}
				require.Equal(t, want[radius][vis], postops.CanSeePost(post, radius))

				// a draft is the author's only, whatever its visibility
				draft := &model.Post{VisibilityRadius: vis}
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
