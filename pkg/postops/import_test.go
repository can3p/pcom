package postops

import (
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

func TestExportImport(t *testing.T) {
	testCases := []struct {
		name    string
		post    *core.Post
		wantErr bool
	}{
		{
			name: "post with subject",
			post: &core.Post{
				ID:      uuid.NewString(),
				Subject: null.StringFrom("test subject"),
				Body: `This is a test *post*

with some *markdown* in it`,
				PublishedAt:      null.TimeFrom(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
				VisibilityRadius: core.PostVisibilityDirectOnly,
			},
		},
		{
			name: "post without subject",
			post: &core.Post{
				ID:      uuid.NewString(),
				Subject: null.String{},
				Body: `This is a test *post*

with some *markdown* in it`,
				PublishedAt:      null.TimeFrom(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
				VisibilityRadius: core.PostVisibilityDirectOnly,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := SerializePost(tc.post)
			imported, err := DeserializePost(b)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.post, imported)
		})
	}
}
