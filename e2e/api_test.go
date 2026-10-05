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
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/web"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

// envelope unwraps ginhelpers.API's success shape: {"data": <payload>}.
type envelope[T any] struct {
	Data T `json:"data"`
}

func decodeData[T any](t *testing.T, body string) T {
	t.Helper()

	var env envelope[T]
	require.NoError(t, json.Unmarshal([]byte(body), &env))

	return env.Data
}

// TestAPI_BearerToken tests the Authorization header handling in
// pkg/auth.AuthAPI: a missing header, a malformed scheme or shape, and a
// well-formed but unknown bearer key.
func TestAPI_BearerToken(t *testing.T) {
	app := e2e.Start(t)

	// the feed token is read-only and must not open the API either
	feedUser := newUser(t, app)
	feedToken, err := repo.RegenerateFeedToken(context.Background(), app.DB, feedUser.ID)
	require.NoError(t, err)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"feed_token", "Bearer " + feedToken.Token, http.StatusForbidden},
		{"missing", "", http.StatusBadRequest},
		{"bearer_no_key", "Bearer", http.StatusBadRequest},
		{"unknown_key", fmt.Sprintf("Bearer %s", uuid.NewString()), http.StatusForbidden},
		{"basic_scheme", "Basic x", http.StatusBadRequest},
		{"bearer_extra_token", "Bearer a b", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := app.Client(t)

			req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
			require.NoError(t, err)

			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			resp := client.Do(req)
			resp.RequireStatus(tc.want)
		})
	}
}

// TestAPI_GetPosts_Empty tests GET /api/v1/posts with an empty posts list,
// even when another user has a published post: pins the UserID filter in
// pkg/web/api.go ApiGetPosts.
func TestAPI_GetPosts_Empty(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	other, err := factory.User(ctx, app.DB)
	require.NoError(t, err)
	_, err = factory.Post(ctx, app.DB, other.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	require.NoError(t, err)

	client := app.Client(t)
	req, err := http.NewRequest(http.MethodGet, app.URL+"/api/v1/posts", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := decodeData[web.ApiGetPostsResponse](t, resp.Body)
	require.Empty(t, result.Posts)
	require.Empty(t, result.Cursor)
}

// TestAPI_GetPosts_Pagination tests that GET /api/v1/posts pages through more
// posts than fit in a single page, following the returned cursor, without
// duplicates or gaps.
func TestAPI_GetPosts_Pagination(t *testing.T) {
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

		result := decodeData[web.ApiGetPostsResponse](t, resp.Body)
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

// TestAPI_GetPosts_LimitClamping tests that a limit above GetPostsLimitMax is
// clamped, and that paging continues correctly past the clamp.
func TestAPI_GetPosts_LimitClamping(t *testing.T) {
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

	result := decodeData[web.ApiGetPostsResponse](t, resp.Body)
	require.Len(t, result.Posts, web.GetPostsLimitMax)
	require.NotEmpty(t, result.Cursor)

	req2, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/posts?limit=100000&cursor=%s", app.URL, result.Cursor), nil)
	require.NoError(t, err)
	req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))

	resp2 := client.Do(req2)
	resp2.RequireStatus(http.StatusOK)

	result2 := decodeData[web.ApiGetPostsResponse](t, resp2.Body)
	require.Len(t, result2.Posts, total-web.GetPostsLimitMax)
	require.Empty(t, result2.Cursor)
}

// TestAPI_NewPost tests POST /api/v1/posts to create a new post.
func TestAPI_NewPost(t *testing.T) {
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

	result := decodeData[web.ApiNewPostResponse](t, resp.Body)
	require.NotEmpty(t, result.ID)
	require.NotEmpty(t, result.PublicURL)

	post, err := factory.GetPost(ctx, app.DB, result.ID)
	require.NoError(t, err)
	require.Equal(t, "Test Subject", lo.FromPtr(post.Subject))
	require.Equal(t, "# Test Body", post.Body)
	require.Equal(t, user.ID, post.UserID)
	require.Equal(t, model.PostVisibilityPublic, post.VisibilityRadius)
	require.Nil(t, post.PublishedAt)

	publishedData := map[string]any{
		"subject":      "Test Subject Published",
		"md_body":      "# Test Body",
		"visibility":   "public",
		"is_published": true,
	}

	publishedBody, err := json.Marshal(publishedData)
	require.NoError(t, err)

	publishedReq, err := http.NewRequest(http.MethodPost, app.URL+"/api/v1/posts", bytes.NewReader(publishedBody))
	require.NoError(t, err)
	publishedReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey.APIKey))
	publishedReq.Header.Set("Content-Type", "application/json")

	publishedResp := client.Do(publishedReq)
	publishedResp.RequireStatus(http.StatusOK)

	publishedResult := decodeData[web.ApiNewPostResponse](t, publishedResp.Body)

	publishedPost, err := factory.GetPost(ctx, app.DB, publishedResult.ID)
	require.NoError(t, err)
	require.NotNil(t, publishedPost.PublishedAt)
}

// TestAPI_EditPost tests POST /api/v1/posts/:id to edit an existing post.
func TestAPI_EditPost(t *testing.T) {
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

	result := decodeData[web.ApiNewPostResponse](t, resp.Body)
	require.Equal(t, post.ID, result.ID)

	got, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Subject", lo.FromPtr(got.Subject))
	require.Equal(t, "# Updated Body", got.Body)
}

// TestAPI_EditPost_Foreign tests POST /api/v1/posts/:id for someone else's
// post (must return 404, and the post must be left unchanged). Pins the
// UserID filter in forms.EditPostFormNew.
func TestAPI_EditPost_Foreign(t *testing.T) {
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

	postData := map[string]any{
		"subject":      "Hijacked Subject",
		"md_body":      "# Hijacked Body",
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
	resp.RequireStatus(http.StatusNotFound)

	got, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, lo.FromPtr(post.Subject), lo.FromPtr(got.Subject))
	require.Equal(t, post.Body, got.Body)
	require.Equal(t, post.VisibilityRadius, got.VisibilityRadius)
}

// TestAPI_DeletePost_Own tests DELETE /api/v1/posts/:id for one's own post.
func TestAPI_DeletePost_Own(t *testing.T) {
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

// TestAPI_MalformedPostID: a post id that is not a UUID is a 404 for edit and delete.
func TestAPI_MalformedPostID(t *testing.T) {
	app := e2e.Start(t)
	user, err := factory.User(context.Background(), app.DB)
	require.NoError(t, err)
	apiKey, err := factory.APIKey(context.Background(), app.DB, user.ID)
	require.NoError(t, err)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequest(method, app.URL+"/api/v1/posts/not-a-uuid", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+apiKey.APIKey)

			app.Client(t).Do(req).RequireStatus(http.StatusNotFound)
		})
	}
}

// TestAPI_DeletePost_Foreign tests DELETE /api/v1/posts/:id for someone else's
// post (must return 404, not leak that the post exists). Fixed in #118.
func TestAPI_DeletePost_Foreign(t *testing.T) {
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

// testImage generates a PNG image (2400x1800) strictly larger than
// every configured media class ("full" is 1200x900, "thumb" is 720x540), so
// every class must actually shrink it.
func testImage() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2400, 1800))
	buf := new(bytes.Buffer)
	_ = png.Encode(buf, img)
	return buf.Bytes()
}

// uploadImage uploads the fixture image over the API and returns the fname
// the server assigned to it.
func uploadImage(t *testing.T, app *e2e.App, client *e2e.Client, apiKey string) string {
	t.Helper()

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", "test.png")
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(testImage()))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPut, app.URL+"/api/v1/image", buf)
	require.NoError(t, err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp := client.Do(req)
	resp.RequireStatus(http.StatusOK)

	result := decodeData[web.ApiUploadImageResponse](t, resp.Body)
	require.NotEmpty(t, result.ImageID)

	return result.ImageID
}

// TestAPI_UploadImage tests PUT /api/v1/image, then fetches the resized
// classes of the uploaded image through /user-media and checks their width,
// black-box: upload, GET, decode.
func TestAPI_UploadImage(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := app.Client(t)

	fname := uploadImage(t, app, client, apiKey.APIKey)

	// the upload is stored as is, under its fname
	require.Equal(t, "image/png", app.S3Object(t, fname).ContentType)

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

	// fetching a class stores its resized variant next to the upload
	for _, tc := range cases {
		require.Equal(t, "image/webp", app.ResizedVariant(t, fname, tc.class).ContentType, "class %s", tc.class)
	}
}

// TestAPI_UploadImage_UnknownName tests that fetching an unknown fname through
// /user-media returns 404.
func TestAPI_UploadImage_UnknownName(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	app.Client(t).Get("/user-media/00000000-0000-0000-0000-000000000000.png/full").RequireStatus(http.StatusNotFound)
}

// TestAPI_RSSPrivate_Valid tests GET /rss/private/:key with a valid API key.
// The route renders the key owner's feed (web.Feed), which includes a direct
// connection's published post regardless of visibility, but never an
// unrelated user's post, however public: seed one of each and check which
// subject shows up.
func TestAPI_RSSPrivate_Valid(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	feedToken, err := repo.RegenerateFeedToken(ctx, app.DB, user.ID)
	require.NoError(t, err)

	direct, err := factory.User(ctx, app.DB)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, user.ID, direct.ID)
	require.NoError(t, err)
	directPost, err := factory.Post(ctx, app.DB, direct.ID, factory.Published())
	require.NoError(t, err)

	unrelated, err := factory.User(ctx, app.DB)
	require.NoError(t, err)
	unrelatedPost, err := factory.Post(ctx, app.DB, unrelated.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	require.NoError(t, err)

	client := app.Client(t)

	resp := client.Get(fmt.Sprintf("/rss/private/%s", feedToken.Token))
	resp.RequireStatus(http.StatusOK)

	require.Contains(t, resp.Body, "<?xml")
	require.Contains(t, resp.Body, "<rss")
	require.Contains(t, resp.Body, lo.FromPtr(directPost.Subject))
	require.NotContains(t, resp.Body, lo.FromPtr(unrelatedPost.Subject))
}

// TestAPI_RSSPrivate_Refused: what /rss/private/:token refuses. The API key can
// write, so it does not open a feed: like any unknown token it is a 404.
func TestAPI_RSSPrivate_Refused(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user, err := factory.User(ctx, app.DB)
	require.NoError(t, err)
	apiKey, err := factory.APIKey(ctx, app.DB, user.ID)
	require.NoError(t, err)

	cases := []struct {
		name string
		key  string
		want int
		text string
	}{
		{"malformed", "unknown-key-12345", http.StatusNotFound, ""},
		{"unknown_uuid", uuid.NewString(), http.StatusNotFound, ""},
		{"api_key", apiKey.APIKey, http.StatusNotFound, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := app.Client(t).Get("/rss/private/" + tc.key)

			require.Equal(t, tc.want, resp.StatusCode)
			require.Contains(t, resp.Body, tc.text)
		})
	}
}

// TestAPI_RSSPrivate_Regenerate: regenerating the token through the settings
// action stops the old feed URL and opens the new one.
func TestAPI_RSSPrivate_Regenerate(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	user := newUser(t, app)
	old, err := repo.RegenerateFeedToken(ctx, app.DB, user.ID)
	require.NoError(t, err)

	client := loginAs(t, app, user)
	client.Get("/rss/private/" + old.Token).RequireStatus(http.StatusOK)

	client.PostJSON("/controls/action/regenerate_feed_token", map[string]string{}).RequireStatus(http.StatusOK)

	current, err := repo.FeedTokenForUser(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.NotEqual(t, old.Token, current.Token)

	app.Client(t).Get("/rss/private/" + old.Token).RequireStatus(http.StatusNotFound)
	app.Client(t).Get("/rss/private/" + current.Token).RequireStatus(http.StatusOK)
}
