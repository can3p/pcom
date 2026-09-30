package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// requestContext returns a builder for a *gin.Context for method against a
// target (which may include a query string), with no body.
func requestContext(method string) func(target string) *gin.Context {
	return func(target string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(method, target, nil)
		return c
	}
}

// jsonContext builds a *gin.Context for a POST/PUT with a JSON body, the
// way c.BindJSON reads it (it ignores the Content-Type header).
func jsonContext(t *testing.T, target string, body any) *gin.Context {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, target, bytes.NewReader(raw))

	return c
}

// multipartFileContext builds a *gin.Context for a POST carrying a single
// multipart file field named "file", the way c.FormFile reads it. An empty
// fieldName skips attaching any file, to exercise the missing-file path.
func multipartFileContext(t *testing.T, fieldName string, data []byte) *gin.Context {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if fieldName != "" {
		part, err := w.CreateFormFile(fieldName, "upload.bin")
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/media", &buf)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())

	return c
}

// postsService is the posts service over db and a sender that records
// instead of sending.
func postsService(db *sqlx.DB, s sender.Sender) *posts.Service {
	return registry.New(db, registry.Deps{Sender: s}).Posts
}

func formatUnix(u int64) string {
	return strconv.FormatInt(u, 10)
}

func TestApiDeletePost(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	t.Run("unknown id", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, testDB.DB))(t)

		c := requestContext(http.MethodDelete)("/api/v1/posts/unknown")
		res := web.ApiDeletePost(c, postsService(testDB.DB, nil), author, "0190a0a0-0000-7000-8000-000000000000")
		require.True(t, res.IsError())
		require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	})

	t.Run("author can delete own post", func(t *testing.T) {
		t.Parallel()

		author := testutil.Must(factory.User(ctx, testDB.DB))(t)
		post := testutil.Must(factory.Post(ctx, testDB.DB, author.ID))(t)

		c := requestContext(http.MethodDelete)("/api/v1/posts/" + post.ID)
		res := web.ApiDeletePost(c, postsService(testDB.DB, nil), author, post.ID)
		require.NoError(t, res.Error())

		_, err := factory.GetPost(ctx, testDB.DB, post.ID)
		require.Error(t, err, "the post must actually be removed")
	})
}

func TestApiGetPosts_LimitClamping(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)
	testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)
	testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)

	newCtx := requestContext(http.MethodGet)

	// limit <= 0 is clamped up to 1, not treated as "no limit": with two
	// posts available, a request that didn't clamp would return both.
	res := web.ApiGetPosts(newCtx("/api/v1/posts?limit=0"), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.NotEmpty(t, resp.Cursor, "more posts exist than the clamped page, so a cursor must be filled")
}

func TestApiGetPosts_Cursor(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)
	p1 := testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)
	p2 := testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)
	p3 := testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)

	newCtx := requestContext(http.MethodGet)

	res := web.ApiGetPosts(newCtx("/api/v1/posts?limit=2"), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 2)
	require.Equal(t, p3.ID, resp.Posts[0].ID)
	require.Equal(t, p2.ID, resp.Posts[1].ID)
	require.Equal(t, p2.ID, resp.Cursor)

	res = web.ApiGetPosts(newCtx("/api/v1/posts?limit=2&cursor="+resp.Cursor), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, p1.ID, resp.Posts[0].ID)
	require.Empty(t, resp.Cursor)
}

func TestApiGetPosts_UpdatedSince(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)
	post := testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)

	newCtx := requestContext(http.MethodGet)

	// A threshold far in the past includes the post, and one far in the
	// future excludes it. These stay clear of the known boundary bug
	// below (a few hours either way don't flip either outcome).
	longAgo := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	res := web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(longAgo)), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, post.ID, resp.Posts[0].ID)

	farFuture := time.Now().Add(24 * time.Hour).Unix()
	res = web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(farFuture)), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Empty(t, resp.Posts)
}

func TestApiGetPosts_UpdatedSince_Boundary(t *testing.T) {
	// Not t.Parallel(): this test forces the process's local zone so the
	// bug reproduces deterministically regardless of the host's real zone.
	origLocal := time.Local
	time.Local = time.FixedZone("UTC+2", 7200)
	t.Cleanup(func() { time.Local = origLocal })

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)
	post := testutil.Must(factory.Post(ctx, testDB.DB, user.ID))(t)
	saved := testutil.Must(factory.GetPost(ctx, testDB.DB, post.ID))(t)

	newCtx := requestContext(http.MethodGet)

	before := saved.UpdatedAt.Time.Add(-time.Minute).Unix()
	res := web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(before)), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, post.ID, resp.Posts[0].ID)

	after := saved.UpdatedAt.Time.Add(time.Minute).Unix()
	res = web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(after)), postsService(testDB.DB, nil), user)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Empty(t, resp.Posts)
}

func TestApiNewPost(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	cases := []struct {
		name        string
		isPublished bool
	}{
		{"publish", true},
		{"draft", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			author := testutil.Must(factory.User(ctx, testDB.DB))(t)
			friend := testutil.Must(factory.User(ctx, testDB.DB))(t)
			connect(t, testDB.DB, ctx, author.ID, friend.ID)

			sender := fakesender.New()

			c := jsonContext(t, "/api/v1/posts", &web.ApiPost{
				Subject:     "Hello world",
				MdBody:      "some **body**",
				Visibility:  core.PostVisibilityPublic,
				IsPublished: tc.isPublished,
			})

			res := web.ApiNewPost(c, postsService(testDB.DB, sender), author)
			require.True(t, res.IsOk())
			resp := res.MustGet()
			require.NotEmpty(t, resp.ID)

			post := testutil.Must(factory.GetPost(ctx, testDB.DB, resp.ID))(t)
			require.Equal(t, tc.isPublished, post.PublishedAt.Valid)
			require.Equal(t, "some **body**", post.Body)

			if tc.isPublished {
				require.Equal(t, links.AbsLink("post", resp.ID), resp.PublicURL)
				require.Equal(t, core.PostVisibilityPublic, post.VisibilityRadius)

				sent := sender.Sent()
				require.Len(t, sent, 1, "publishing should notify the author's direct connections")
				require.Equal(t, "post_notification", sent[0].EmailType)
				require.Equal(t, friend.Email, sent[0].Mail.To[0].Address)
			} else {
				require.Empty(t, sender.Sent(), "a draft must not notify anybody")
			}
		})
	}
}

func TestApiEditPost_PublishAndMakeDraft(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, testDB.DB))(t)
	friend := testutil.Must(factory.User(ctx, testDB.DB))(t)
	connect(t, testDB.DB, ctx, author.ID, friend.ID)

	post := testutil.Must(factory.Post(ctx, testDB.DB, author.ID))(t)
	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts/"+post.ID, &web.ApiPost{
		Subject:     "Edited subject",
		MdBody:      "edited body",
		Visibility:  core.PostVisibilityPublic,
		IsPublished: true,
	})

	res := web.ApiEditPost(c, postsService(testDB.DB, sender), author, post.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Equal(t, post.ID, resp.ID)

	got := testutil.Must(factory.GetPost(ctx, testDB.DB, post.ID))(t)
	require.True(t, got.PublishedAt.Valid, "publish must set the post live")
	require.Equal(t, "edited body", got.Body)

	sent := sender.Sent()
	require.Len(t, sent, 1, "publishing an edit should notify the author's direct connections")
	require.Equal(t, friend.Email, sent[0].Mail.To[0].Address)

	// Editing again with is_published=false moves the post back to draft
	// and does not send a second round of notifications.
	c = jsonContext(t, "/api/v1/posts/"+post.ID, &web.ApiPost{
		Subject:     "Edited subject",
		MdBody:      "edited body",
		Visibility:  core.PostVisibilityPublic,
		IsPublished: false,
	})

	res = web.ApiEditPost(c, postsService(testDB.DB, sender), author, post.ID)
	require.True(t, res.IsOk())

	got = testutil.Must(factory.GetPost(ctx, testDB.DB, post.ID))(t)
	require.False(t, got.PublishedAt.Valid, "is_published=false must move the post back to draft")
	require.Len(t, sender.Sent(), 1, "moving back to draft must not send another notification")
}

func TestApiEditPost_UnknownPost(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	author := testutil.Must(factory.User(ctx, testDB.DB))(t)
	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts/unknown", &web.ApiPost{
		Subject: "x",
		MdBody:  "y",
	})

	res := web.ApiEditPost(c, postsService(testDB.DB, sender), author, "0190a0a0-0000-7000-8000-000000000000")
	require.True(t, res.IsError())
	require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
}

// pngBytes is a minimal, valid one-pixel PNG file, enough for
// http.DetectContentType to report "image/png".
var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D,
	0xB0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

func TestApiUploadImage(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, testDB.DB))(t)

	cases := []struct {
		name      string
		fieldName string
		data      []byte
		wantOk    bool
		wantErr   error
	}{
		{"success", "file", pngBytes, true, nil},
		{"missing file", "", nil, false, nil},
		{"unsupported type", "file", []byte("just some plain text, not an image"), false, media.ErrUnsupportedMimeType},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage := fakestorage.New()
			c := multipartFileContext(t, tc.fieldName, tc.data)

			res := web.ApiUploadImageWith(c, posts.New(repo.New(testDB.DB), nil, storage), user)

			if !tc.wantOk {
				require.True(t, res.IsError())
				if tc.wantErr != nil {
					require.ErrorIs(t, res.Error(), tc.wantErr)
				}
				return
			}

			resp := res.MustGet()
			require.NotEmpty(t, resp.ImageID)

			exists, err := storage.ObjectExists(ctx, resp.ImageID)
			require.NoError(t, err)
			require.True(t, exists, "the uploaded file must land in storage")
		})
	}
}
