package postops

import (
	"context"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

func TestImport(t *testing.T) {
	testCases := []struct {
		name             string
		content          string
		post             *core.Post
		additionalFields *AdditionalFields
		wantErr          bool
	}{
		{
			name: "post with subject",
			content: `---
original_id: 018f45ef-b63a-7426-a444-3957146ca700
subject: test subject
published: 2024-05-04T23:28:08Z
visibility: direct_only
---

This is a test *post*

with some *markdown* in it`,
			post: &core.Post{
				ID:      "018f45ef-b63a-7426-a444-3957146ca700",
				Subject: null.StringFrom("test subject"),
				Body: `This is a test *post*

with some *markdown* in it`,
				PublishedAt:      null.TimeFrom(time.Date(2024, time.May, 4, 23, 28, 8, 0, time.UTC)),
				VisibilityRadius: core.PostVisibilityDirectOnly,
			},
		},
		{
			name: "post with empty subject",
			content: `---
			original_id: 018f45ef-b63a-7426-a444-3957146ca700
subject: 
published: 2024-05-04T23:28:08Z
visibility: direct_only
---

This is a test *post*

with some *markdown* in it`,
			post: &core.Post{
				ID: "018f45ef-b63a-7426-a444-3957146ca700",
				Body: `This is a test *post*

with some *markdown* in it`,
				PublishedAt:      null.TimeFrom(time.Date(2024, time.May, 4, 23, 28, 8, 0, time.UTC)),
				VisibilityRadius: core.PostVisibilityDirectOnly,
			},
		},
		{
			name: "post with URL",
			content: `---
			original_id: 018f45ef-b63a-7426-a444-3957146ca700
			url: https://test.url
published: 2024-05-04T23:28:08Z
visibility: direct_only
---

This is a test *post*

with some *markdown* in it`,
			post: &core.Post{
				ID: "018f45ef-b63a-7426-a444-3957146ca700",
				Body: `This is a test *post*

with some *markdown* in it`,
				PublishedAt:      null.TimeFrom(time.Date(2024, time.May, 4, 23, 28, 8, 0, time.UTC)),
				VisibilityRadius: core.PostVisibilityDirectOnly,
			},
			additionalFields: &AdditionalFields{
				URL: "https://test.url",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imported, additionalFields, err := DeserializePost([]byte(tc.content))

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.post, imported)
			assert.Equal(t, tc.additionalFields, additionalFields)
		})
	}
}
func TestExportImport(t *testing.T) {
	testCases := []struct {
		name             string
		post             *core.Post
		additionalFields *AdditionalFields
		wantErr          bool
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
		{
			name: "post with URL",
			post: func() *core.Post {
				p := &core.Post{
					ID:               uuid.NewString(),
					Subject:          null.StringFrom("test subject with URL"),
					Body:             `This is a test *post* with URL`,
					PublishedAt:      null.TimeFrom(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
					VisibilityRadius: core.PostVisibilityDirectOnly,
					URLID:            null.StringFrom("test-url-id"),
				}

				p.R = p.R.NewStruct()
				p.R.URL = &core.NormalizedURL{
					URL: "https://example.com",
				}

				return p
			}(),
			additionalFields: &AdditionalFields{
				URL: "https://example.com",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := SerializePost(tc.post)
			imported, additionalFields, err := DeserializePost(b)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			tc.post.R = nil
			// url id is never filled in in imported post
			tc.post.URLID = null.String{}
			assert.Equal(t, tc.post, imported)
			assert.Equal(t, tc.additionalFields, additionalFields)
		})
	}
}

func TestDeserializeArchive(t *testing.T) {
	post := &core.Post{
		ID:      uuid.NewString(),
		Subject: null.StringFrom("test subject"),
		Body: `This is a test *post*

with some *markdown* in it`,
		PublishedAt:      null.TimeFrom(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
		VisibilityRadius: core.PostVisibilityDirectOnly,
		URLID:            null.StringFrom("test-url-id"),
	}

	post.R = post.R.NewStruct()
	post.R.URL = &core.NormalizedURL{
		URL: "https://example.com",
	}

	b, err := SerializeBlogSlice(context.Background(), []*core.Post{post}, nil)
	assert.NoError(t, err)

	posts, images, err := DeserializeArchive(b)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(posts))
	assert.Equal(t, 0, len(images))

	// Clear R field before comparison as it's not part of serialization
	post.R = nil
	post.URLID = null.String{}
	assert.Equal(t, post, posts[0].Post)
	assert.Equal(t, "https://example.com", posts[0].Additional.URL)
}
