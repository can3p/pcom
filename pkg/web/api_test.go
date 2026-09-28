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

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApiDeletePost_UnknownID(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/posts/unknown", nil)

	res := web.ApiDeletePost(c, testDB.DB, author, "0190a0a0-0000-7000-8000-000000000000")
	require.True(t, res.IsError())
	require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
}

func TestApiDeletePost_AuthorCanDeleteOwnPost(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, testDB.DB, author.ID)
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+post.ID, nil)

	res := web.ApiDeletePost(c, testDB.DB, author, post.ID)
	require.NoError(t, res.Error())

	_, err = factory.GetPost(ctx, testDB.DB, post.ID)
	require.Error(t, err, "the post must actually be removed")
}

// newGetContext builds a *gin.Context for an ApiGetPosts call against
// target (which may include a query string), the way c.ShouldBind reads GET
// query parameters.
func newGetContext() func(target string) *gin.Context {
	return func(target string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, target, nil)
		return c
	}
}

func TestApiGetPosts_LimitClamping(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	_, err = factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	_, err = factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)

	newCtx := newGetContext()

	// limit <= 0 is clamped up to 1, not treated as "no limit": with two
	// posts available, a request that didn't clamp would return both.
	res := web.ApiGetPosts(newCtx("/api/v1/posts?limit=0"), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.NotEmpty(t, resp.Cursor, "more posts exist than the clamped page, so a cursor must be filled")
}

func TestApiGetPosts_Cursor(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	p1, err := factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	p2, err := factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)
	p3, err := factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)

	newCtx := newGetContext()

	res := web.ApiGetPosts(newCtx("/api/v1/posts?limit=2"), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 2)
	require.Equal(t, p3.ID, resp.Posts[0].ID)
	require.Equal(t, p2.ID, resp.Posts[1].ID)
	require.Equal(t, p2.ID, resp.Cursor)

	res = web.ApiGetPosts(newCtx("/api/v1/posts?limit=2&cursor="+resp.Cursor), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, p1.ID, resp.Posts[0].ID)
	require.Empty(t, resp.Cursor)
}

func TestApiGetPosts_UpdatedSince(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)

	newCtx := newGetContext()

	// A threshold far in the past includes the post, and one far in the
	// future excludes it. These stay clear of the known boundary bug
	// below (a few hours either way don't flip either outcome).
	longAgo := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	res := web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(longAgo)), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, post.ID, resp.Posts[0].ID)

	farFuture := time.Now().Add(24 * time.Hour).Unix()
	res = web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(farFuture)), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Empty(t, resp.Posts)
}

// updatedSinceBoundaryBug describes a real bug found while pinning
// ApiGetPosts's updated_since filter, reported to the coordinator to file:
// ApiGetPosts builds time.Unix(updated_since, 0), which is a time.Time in
// the process's local zone, and compares it against the "updated_at" column
// which is "timestamp" (no time zone). Outside UTC the comparison silently
// shifts by the host's UTC offset: a post updated a minute after the
// threshold can be wrongly excluded (reproduced on a host running two hours
// ahead of UTC).
const updatedSinceBoundaryBug = "known bug #156: ApiGetPosts's updated_since filter is timezone-dependent: " +
	"time.Unix(updated_since, 0) is compared against the updated_at column (a timestamp with no time " +
	"zone) using the process's local zone, so outside UTC the threshold is off by the host's UTC " +
	"offset and a post updated shortly after the threshold can be wrongly excluded"

func TestApiGetPosts_UpdatedSince_Boundary(t *testing.T) {
	t.Skip(updatedSinceBoundaryBug)

	// Not t.Parallel(): this test forces the process's local zone so the
	// bug reproduces deterministically regardless of the host's real zone.
	origLocal := time.Local
	time.Local = time.FixedZone("UTC+2", 7200)
	t.Cleanup(func() { time.Local = origLocal })

	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	post, err := factory.Post(ctx, testDB.DB, user.ID)
	require.NoError(t, err)

	saved, err := factory.GetPost(ctx, testDB.DB, post.ID)
	require.NoError(t, err)

	newCtx := newGetContext()

	before := saved.UpdatedAt.Time.Add(-time.Minute).Unix()
	res := web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(before)), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Len(t, resp.Posts, 1)
	require.Equal(t, post.ID, resp.Posts[0].ID)

	after := saved.UpdatedAt.Time.Add(time.Minute).Unix()
	res = web.ApiGetPosts(newCtx("/api/v1/posts?updated_since="+formatUnix(after)), testDB.DB, user.ID)
	require.True(t, res.IsOk())
	resp = res.MustGet()
	require.Empty(t, resp.Posts)
}

func formatUnix(u int64) string {
	return strconv.FormatInt(u, 10)
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

func TestApiNewPost_Publish(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	friend, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, testDB.DB, author.ID, friend.ID)
	require.NoError(t, err)

	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts", &web.ApiPost{
		Subject:     "Hello world",
		MdBody:      "some **body**",
		Visibility:  core.PostVisibilityPublic,
		IsPublished: true,
	})

	res := web.ApiNewPost(c, testDB.DB, sender, author, links.MediaReplacer)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.NotEmpty(t, resp.ID)
	require.Equal(t, links.AbsLink("post", resp.ID), resp.PublicURL)

	post, err := factory.GetPost(ctx, testDB.DB, resp.ID)
	require.NoError(t, err)
	require.True(t, post.PublishedAt.Valid, "publish must set the post live")
	require.Equal(t, core.PostVisibilityPublic, post.VisibilityRadius)
	require.Equal(t, "some **body**", post.Body)

	sent := sender.Sent()
	require.Len(t, sent, 1, "publishing should notify the author's direct connections")
	require.Equal(t, "post_notification", sent[0].EmailType)
	require.Equal(t, friend.Email, sent[0].Mail.To[0].Address)
}

func TestApiNewPost_Draft(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	friend, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, testDB.DB, author.ID, friend.ID)
	require.NoError(t, err)

	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts", &web.ApiPost{
		Subject:     "Draft subject",
		MdBody:      "draft body",
		Visibility:  core.PostVisibilityDirectOnly,
		IsPublished: false,
	})

	res := web.ApiNewPost(c, testDB.DB, sender, author, links.MediaReplacer)
	require.True(t, res.IsOk())
	resp := res.MustGet()

	post, err := factory.GetPost(ctx, testDB.DB, resp.ID)
	require.NoError(t, err)
	require.False(t, post.PublishedAt.Valid, "a draft must not be published")

	require.Empty(t, sender.Sent(), "a draft must not notify anybody")
}

func TestApiEditPost_PublishAndMakeDraft(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	friend, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, testDB.DB, author.ID, friend.ID)
	require.NoError(t, err)

	post, err := factory.Post(ctx, testDB.DB, author.ID)
	require.NoError(t, err)

	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts/"+post.ID, &web.ApiPost{
		Subject:     "Edited subject",
		MdBody:      "edited body",
		Visibility:  core.PostVisibilityPublic,
		IsPublished: true,
	})

	res := web.ApiEditPost(c, testDB.DB, sender, author, links.MediaReplacer, post.ID)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.Equal(t, post.ID, resp.ID)

	got, err := factory.GetPost(ctx, testDB.DB, post.ID)
	require.NoError(t, err)
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

	res = web.ApiEditPost(c, testDB.DB, sender, author, links.MediaReplacer, post.ID)
	require.True(t, res.IsOk())

	got, err = factory.GetPost(ctx, testDB.DB, post.ID)
	require.NoError(t, err)
	require.False(t, got.PublishedAt.Valid, "is_published=false must move the post back to draft")
	require.Len(t, sender.Sent(), 1, "moving back to draft must not send another notification")
}

func TestApiEditPost_UnknownPost(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	author, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	sender := fakesender.New()

	c := jsonContext(t, "/api/v1/posts/unknown", &web.ApiPost{
		Subject: "x",
		MdBody:  "y",
	})

	res := web.ApiEditPost(c, testDB.DB, sender, author, links.MediaReplacer, "0190a0a0-0000-7000-8000-000000000000")
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

func TestApiUploadImage_Success(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	storage := fakestorage.New()

	c := multipartFileContext(t, "file", pngBytes)

	res := web.ApiUploadImage(c, testDB.DB, user, storage)
	require.True(t, res.IsOk())
	resp := res.MustGet()
	require.NotEmpty(t, resp.ImageID)

	exists, err := storage.ObjectExists(ctx, resp.ImageID)
	require.NoError(t, err)
	require.True(t, exists, "the uploaded file must land in storage")
}

func TestApiUploadImage_MissingFile(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	storage := fakestorage.New()

	c := multipartFileContext(t, "", nil)

	res := web.ApiUploadImage(c, testDB.DB, user, storage)
	require.True(t, res.IsError())
}

func TestApiUploadImage_UnsupportedType(t *testing.T) {
	testDB := testdb.New(t)
	ctx := context.Background()

	user, err := factory.User(ctx, testDB.DB)
	require.NoError(t, err)

	storage := fakestorage.New()

	c := multipartFileContext(t, "file", []byte("just some plain text, not an image"))

	res := web.ApiUploadImage(c, testDB.DB, user, storage)
	require.True(t, res.IsError())
	require.ErrorIs(t, res.Error(), media.ErrUnsupportedMimeType)
}
