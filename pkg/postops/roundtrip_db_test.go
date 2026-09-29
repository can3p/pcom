package postops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// TestSerializeBlog_RoundTripAcrossUsers exports one user's blog and imports
// it into a different user, then compares the imported posts against the
// originals: this is the export/import path a user goes through when moving
// (or backing up and restoring into) an account.
func TestSerializeBlog_RoundTripAcrossUsers(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	author := testutil.Must(factory.User(ctx, db))(t)
	newOwner := testutil.Must(factory.User(ctx, db))(t)
	link := testutil.Must(factory.NormalizedURL(ctx, db))(t)

	withLink := testutil.Must(factory.Post(ctx, db, author.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityPublic),
		factory.WithURL(link.ID),
	))(t)
	plain := testutil.Must(factory.Post(ctx, db, author.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityDirectOnly),
	))(t)

	// Posts with a "timestamp without time zone" column round-trip through
	// Postgres losing their original zone, so compare against a value read
	// back from the DB rather than the in-memory one factory.Post returned.
	dbWithLink := testutil.Must(factory.GetPost(ctx, db, withLink.ID))(t)

	archive := testutil.Must(postops.SerializeBlog(ctx, db, storage, author.ID))(t)

	posts, images, err := postops.DeserializeArchive(archive)
	require.NoError(t, err)
	require.Len(t, posts, 2)
	require.Empty(t, images)

	stats := testutil.Must(postops.InjectPostsInDB(ctx, db, storage, newOwner.ID, posts, images))(t)
	require.Equal(t, 2, stats.PostsCreated)
	require.Equal(t, 0, stats.PostsUpdated)

	imported := testutil.Must(factory.ListPosts(ctx, db, newOwner.ID))(t)
	require.Len(t, imported, 2)

	bySubject := map[string]*core.Post{}
	for _, p := range imported {
		bySubject[p.Subject.String] = p
	}

	gotWithLink, ok := bySubject[withLink.Subject.String]
	require.True(t, ok)
	require.Equal(t, withLink.Body, gotWithLink.Body)
	require.Equal(t, core.PostVisibilityPublic, gotWithLink.VisibilityRadius)
	require.NotEqual(t, withLink.ID, gotWithLink.ID, "the imported post gets a new id")
	require.Equal(t, newOwner.ID, gotWithLink.UserID)
	require.True(t, gotWithLink.URLID.Valid)
	require.Equal(t, link.ID, gotWithLink.URLID.String, "the normalized url row is shared, not duplicated")
	require.WithinDuration(t, dbWithLink.PublishedAt.Time, gotWithLink.PublishedAt.Time, time.Second)

	gotPlain, ok := bySubject[plain.Subject.String]
	require.True(t, ok)
	require.Equal(t, plain.Body, gotPlain.Body)
	require.Equal(t, core.PostVisibilityDirectOnly, gotPlain.VisibilityRadius)
	require.False(t, gotPlain.URLID.Valid)
}

// TestInjectPostsInDB exercises the import side on its own: updating an
// existing post on self-restore, and the image upload/skip decision.
func TestInjectPostsInDB(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("updates an existing post on self-restore", func(t *testing.T) {
		t.Parallel()

		storage := fakestorage.New()
		author := testutil.Must(factory.User(ctx, db))(t)
		post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Published()))(t)

		// The export carries the original post ID, so importing it back for
		// the same author updates the existing row instead of creating a
		// duplicate.
		archive := testutil.Must(postops.SerializeBlog(ctx, db, storage, author.ID))(t)
		posts, images, err := postops.DeserializeArchive(archive)
		require.NoError(t, err)
		require.Len(t, posts, 1)

		posts[0].Post.Body = "edited body"

		stats := testutil.Must(postops.InjectPostsInDB(ctx, db, storage, author.ID, posts, images))(t)
		require.Equal(t, 0, stats.PostsCreated)
		require.Equal(t, 1, stats.PostsUpdated)

		got := testutil.Must(factory.GetPost(ctx, db, post.ID))(t)
		require.Equal(t, "edited body", got.Body)
	})

	t.Run("uploads new images", func(t *testing.T) {
		t.Parallel()

		storage := fakestorage.New()
		owner := testutil.Must(factory.User(ctx, db))(t)

		pngBytes := []byte("\x89PNG\r\n\x1a\nrest-of-file")
		posts := []*postops.PostWithMeta{
			{
				Post: &core.Post{
					Subject:          null.StringFrom("with image"),
					Body:             "![alt](original-name.png)",
					VisibilityRadius: core.PostVisibilityDirectOnly,
				},
			},
		}
		images := map[string][]byte{"original-name.png": pngBytes}

		stats := testutil.Must(postops.InjectPostsInDB(ctx, db, storage, owner.ID, posts, images))(t)
		require.Equal(t, 1, stats.ImagesUploaded)
		require.Equal(t, 0, stats.ImagesSkipped)

		imported := testutil.Must(factory.ListPosts(ctx, db, owner.ID))(t)
		require.Len(t, imported, 1)
		require.NotContains(t, imported[0].Body, "original-name.png",
			"the image reference should be rewritten to the freshly uploaded name")
	})

	t.Run("skips reupload of an existing image", func(t *testing.T) {
		t.Parallel()

		storage := fakestorage.New()
		owner := testutil.Must(factory.User(ctx, db))(t)
		existing := testutil.Must(factory.MediaUpload(ctx, db, owner.ID))(t)

		posts := []*postops.PostWithMeta{
			{
				Post: &core.Post{
					Subject:          null.StringFrom("no new images"),
					Body:             "plain text, no image references",
					VisibilityRadius: core.PostVisibilityDirectOnly,
				},
			},
		}
		images := map[string][]byte{existing.UploadedFname: []byte("does-not-matter")}

		stats := testutil.Must(postops.InjectPostsInDB(ctx, db, storage, owner.ID, posts, images))(t)
		require.Equal(t, 0, stats.ImagesUploaded)
		require.Equal(t, 1, stats.ImagesSkipped)
	})
}
