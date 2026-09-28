package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/web"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

// e4Envelope unwraps ginhelpers.API's success shape: {"data": <payload>}.
type e4Envelope[T any] struct {
	Data T `json:"data"`
}

func e4DecodeData[T any](t *testing.T, body string) T {
	t.Helper()

	var env e4Envelope[T]
	require.NoError(t, json.Unmarshal([]byte(body), &env))

	return env.Data
}

// TestE4_BearerToken_Missing tests API auth with missing bearer token.
func TestE4_BearerToken_Missing(t *testing.T) {
	app := e2e.Start(t)
	client := app.Client(t)

	resp := client.Get("/api/v1/posts")

	resp.RequireStatus(http.StatusBadRequest)
}

// TestE4_BearerToken_Malformed tests API auth with a malformed bearer token
// (no key, just the "Bearer" prefix).
func TestE4_BearerToken_Malformed(t *testing.T) {
	app := e2e.Start(t)
	client := app.Client(t)

	req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer")

	resp := client.Do(req)
	resp.RequireStatus(http.StatusBadRequest)
}

// TestE4_BearerToken_Unknown tests API auth with a well-formed (API keys are
// UUIDs) but unknown bearer key.
func TestE4_BearerToken_Unknown(t *testing.T) {
	app := e2e.Start(t)
	client := app.Client(t)

	req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", uuid.NewString()))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusForbidden)
}

// TestE4_GetPosts_Empty tests GET /api/v1/posts with an empty posts list.
func TestE4_GetPosts_Empty(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)
	req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := e4DecodeData[web.ApiGetPostsResponse](t, resp.Body)
	require.Empty(t, result.Posts)
	require.Empty(t, result.Cursor)
}

// TestE4_GetPosts_Pagination tests that GET /api/v1/posts pages through more
// posts than fit in a single page, following the returned cursor, without
// duplicates or gaps.
func TestE4_GetPosts_Pagination(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	const total = 5
	const pageSize = 2

	want := make(map[string]bool, total)
	for range total {
		post, err := factory.Post(ctx, app.DB, user.ID)
		require.NoError(t, err)
		want[post.ID] = true
	}

	client := app.Client(t)

	got := map[string]bool{}
	cursor := ""

	for range total { // generous upper bound on iterations
		url := fmt.Sprintf("%s/api/v1/posts?limit=%d", app.URL, pageSize)
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

		resp := client.Do(req)
		resp.RequireStatus(http.StatusOK)

		result := e4DecodeData[web.ApiGetPostsResponse](t, resp.Body)
		require.LessOrEqual(t, len(result.Posts), pageSize)

		for _, p := range result.Posts {
			require.False(t, got[p.ID], "post %s returned twice", p.ID)
			got[p.ID] = true
		}

		if result.Cursor == "" {
			break
		}

		cursor = result.Cursor
	}

	require.Equal(t, want, got)
}

// TestE4_GetPosts_LimitClamping tests that a limit above GetPostsLimitMax is
// clamped, and that paging continues correctly past the clamp.
func TestE4_GetPosts_LimitClamping(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	total := web.GetPostsLimitMax + 1
	for range total {
		_, err := factory.Post(ctx, app.DB, user.ID)
		require.NoError(t, err)
	}

	client := app.Client(t)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/posts?limit=100000", app.URL), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := e4DecodeData[web.ApiGetPostsResponse](t, resp.Body)
	require.Len(t, result.Posts, web.GetPostsLimitMax)
	require.NotEmpty(t, result.Cursor)

	req2, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/posts?limit=100000&cursor=%s", app.URL, result.Cursor), nil)
	require.NoError(t, err)
	req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp2 := client.Do(req2)
	resp2.RequireStatus(http.StatusOK)

	result2 := e4DecodeData[web.ApiGetPostsResponse](t, resp2.Body)
	require.Len(t, result2.Posts, total-web.GetPostsLimitMax)
	require.Empty(t, result2.Cursor)
}

// TestE4_NewPost tests POST /api/v1/posts to create a new post.
func TestE4_NewPost(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	postData := map[string]any{
		"subject":      "Test Subject",
		"md_body":      "# Test Body",
		"visibility":   "public",
		"is_published": false,
	}

	body, err := json.Marshal(postData)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, app.URL+"/api/v1/posts", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := e4DecodeData[web.ApiNewPostResponse](t, resp.Body)
	require.NotEmpty(t, result.ID)
	require.NotEmpty(t, result.PublicURL)

	post, err := factory.GetPost(ctx, app.DB, result.ID)
	require.NoError(t, err)
	require.Equal(t, "Test Subject", post.Subject.String)
	require.Equal(t, "# Test Body", post.Body)
	require.Equal(t, user.ID, post.UserID)
}

// TestE4_EditPost tests POST /api/v1/posts/:id to edit an existing post.
func TestE4_EditPost(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	postData := map[string]any{
		"subject":      "Updated Subject",
		"md_body":      "# Updated Body",
		"visibility":   "public",
		"is_published": false,
	}

	body, err := json.Marshal(postData)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/posts/%s", app.URL, post.ID), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := e4DecodeData[web.ApiNewPostResponse](t, resp.Body)
	require.Equal(t, post.ID, result.ID)

	got, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Subject", got.Subject.String)
	require.Equal(t, "# Updated Body", got.Body)
}

// TestE4_DeletePost_Own tests DELETE /api/v1/posts/:id for one's own post.
func TestE4_DeletePost_Own(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/posts/%s", app.URL, post.ID), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	_, err = factory.GetPost(ctx, app.DB, post.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

// TestE4_DeletePost_Foreign tests DELETE /api/v1/posts/:id for someone else's
// post (must return 404, not leak that the post exists). Fixed in #118.
func TestE4_DeletePost_Foreign(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user1, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	user2, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, app.DB, user2.ID)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user1.ID)
	require.NoError(t, err)

	client := app.Client(t)

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/posts/%s", app.URL, post.ID), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusNotFound)

	got, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// e4GenerateTestImage generates a small PNG image (1200x900, matching the
// "full" media class exactly, so that "thumb" is the only class that shrinks
// it).
func e4GenerateTestImage() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	buf := new(bytes.Buffer)
	_ = png.Encode(buf, img)
	return buf.Bytes()
}

// e4UploadImage uploads the fixture image over the API and returns the fname
// the server assigned to it.
func e4UploadImage(t *testing.T, app *e2e.App, client *e2e.Client, apiKey string) string {
	t.Helper()

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", "test.png")
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(e4GenerateTestImage()))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPut, app.URL+"/api/v1/image", buf)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := e4DecodeData[web.ApiUploadImageResponse](t, resp.Body)
	require.NotEmpty(t, result.ImageID)

	return result.ImageID
}

// TestE4_UploadImage tests PUT /api/v1/image, then fetches the resized
// classes of the uploaded image through /user-media and checks their width,
// black-box: upload, GET, decode.
func TestE4_UploadImage(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	fname := e4UploadImage(t, app, client, apiKey.APIKey)

	cases := []struct {
		class     string
		maxWidth  int
		maxHeight int
	}{
		{"full", 1200, 900},
		{"thumb", 720, 540},
	}

	for _, tc := range cases {
		resp := client.Get(fmt.Sprintf("/user-media/%s/%s", fname, tc.class))
		resp.RequireStatus(http.StatusOK)

		cfg, err := webp.DecodeConfig(bytes.NewReader([]byte(resp.Body)))
		require.NoError(t, err, "class %s", tc.class)
		require.LessOrEqual(t, cfg.Width, tc.maxWidth, "class %s", tc.class)
		require.LessOrEqual(t, cfg.Height, tc.maxHeight, "class %s", tc.class)
	}
}

// TestE4_UploadImage_UnknownName tests that fetching an unknown fname through
// /user-media returns 404.
func TestE4_UploadImage_UnknownName(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/154")

	app := e2e.Start(t)

	client := app.Client(t)

	resp := client.Get("/user-media/00000000-0000-0000-0000-000000000000.png/full")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestE4_RSSPrivate_Valid tests GET /rss/private/:key with a valid API key.
func TestE4_RSSPrivate_Valid(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	resp := client.Get(fmt.Sprintf("/rss/private/%s", apiKey.APIKey))
	resp.RequireStatus(http.StatusOK)

	require.Contains(t, resp.Body, "<?xml")
	require.Contains(t, resp.Body, "<rss")
}

// TestE4_RSSPrivate_Unknown tests GET /rss/private/:key with an unknown key.
func TestE4_RSSPrivate_Unknown(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/115")

	app := e2e.Start(t)
	client := app.Client(t)

	resp := client.Get("/rss/private/unknown-key-12345")

	// Should return 404, not 500.
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
