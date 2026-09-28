package postops_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
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

	author, err := factory.User(ctx, db)
	require.NoError(t, err)
	newOwner, err := factory.User(ctx, db)
	require.NoError(t, err)

	link, err := factory.NormalizedURL(ctx, db)
	require.NoError(t, err)

	withLink, err := factory.Post(ctx, db, author.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityPublic),
		factory.WithURL(link.ID),
	)
	require.NoError(t, err)

	plain, err := factory.Post(ctx, db, author.ID,
		factory.Published(),
		factory.Visibility(core.PostVisibilityDirectOnly),
	)
	require.NoError(t, err)

	// Posts with a "timestamp without time zone" column round-trip through
	// Postgres losing their original zone, so compare against a value read
	// back from the DB rather than the in-memory one factory.Post returned.
	dbWithLink, err := factory.GetPost(ctx, db, withLink.ID)
	require.NoError(t, err)

	archive, err := postops.SerializeBlog(ctx, db, storage, author.ID)
	require.NoError(t, err)

	posts, images, err := postops.DeserializeArchive(archive)
	require.NoError(t, err)
	require.Len(t, posts, 2)
	require.Empty(t, images)

	stats, err := postops.InjectPostsInDB(ctx, db, storage, newOwner.ID, posts, images)
	require.NoError(t, err)
	require.Equal(t, 2, stats.PostsCreated)
	require.Equal(t, 0, stats.PostsUpdated)

	imported, err := factory.ListPosts(ctx, db, newOwner.ID)
	require.NoError(t, err)
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

// TestInjectPostsInDB_UpdatesExistingPost pins the self-restore path: the
// export carries the original post ID, so importing it back for the same
// author updates the existing row instead of creating a duplicate.
func TestInjectPostsInDB_UpdatesExistingPost(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	author, err := factory.User(ctx, db)
	require.NoError(t, err)

	post, err := factory.Post(ctx, db, author.ID, factory.Published())
	require.NoError(t, err)

	archive, err := postops.SerializeBlog(ctx, db, storage, author.ID)
	require.NoError(t, err)

	posts, images, err := postops.DeserializeArchive(archive)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	posts[0].Post.Body = "edited body"

	stats, err := postops.InjectPostsInDB(ctx, db, storage, author.ID, posts, images)
	require.NoError(t, err)
	require.Equal(t, 0, stats.PostsCreated)
	require.Equal(t, 1, stats.PostsUpdated)

	got, err := factory.GetPost(ctx, db, post.ID)
	require.NoError(t, err)
	require.Equal(t, "edited body", got.Body)
}

func TestInjectPostsInDB_UploadsNewImages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	owner, err := factory.User(ctx, db)
	require.NoError(t, err)

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

	stats, err := postops.InjectPostsInDB(ctx, db, storage, owner.ID, posts, images)
	require.NoError(t, err)
	require.Equal(t, 1, stats.ImagesUploaded)
	require.Equal(t, 0, stats.ImagesSkipped)

	imported, err := factory.ListPosts(ctx, db, owner.ID)
	require.NoError(t, err)
	require.Len(t, imported, 1)
	require.NotContains(t, imported[0].Body, "original-name.png",
		"the image reference should be rewritten to the freshly uploaded name")
}

func TestInjectPostsInDB_SkipsReuploadOfExistingImage(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	owner, err := factory.User(ctx, db)
	require.NoError(t, err)

	existing, err := factory.MediaUpload(ctx, db, owner.ID)
	require.NoError(t, err)

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

	stats, err := postops.InjectPostsInDB(ctx, db, storage, owner.ID, posts, images)
	require.NoError(t, err)
	require.Equal(t, 0, stats.ImagesUploaded)
	require.Equal(t, 1, stats.ImagesSkipped)
}
