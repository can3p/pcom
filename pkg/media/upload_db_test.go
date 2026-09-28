package media_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
)

var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

// TestHandleUpload_ForUser_StoresRowAndObject checks both halves of a
// successful upload: the object lands in storage under the returned name,
// and the media_uploads row is linked to the user, not to a feed.
func TestHandleUpload_ForUser_StoresRowAndObject(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	owner, err := factory.User(ctx, db)
	require.NoError(t, err)

	fname, err := media.HandleUpload(ctx, db, storage, &owner.ID, nil, bytes.NewReader(pngBytes))
	require.NoError(t, err)
	require.Contains(t, fname, ".png")

	exists, err := storage.ObjectExists(ctx, fname)
	require.NoError(t, err)
	require.True(t, exists, "the stored object should exist")

	r, _, contentType, err := storage.DownloadFile(ctx, fname)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	require.Equal(t, "image/png", contentType)

	row, err := factory.GetMediaUploadByFname(ctx, db, fname)
	require.NoError(t, err)
	require.True(t, row.UserID.Valid)
	require.Equal(t, owner.ID, row.UserID.String)
	require.False(t, row.RSSFeedID.Valid, "a user upload must not also be linked to a feed")
}

// TestHandleUpload_ForFeed_StoresRowAndObject checks that an upload linked
// to an RSS feed (rather than a user) also lands in storage under the
// returned name, and that the media_uploads row is linked to the feed, not
// to a user.
func TestHandleUpload_ForFeed_StoresRowAndObject(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	storage := fakestorage.New()

	feed, err := factory.RSSFeed(ctx, db)
	require.NoError(t, err)

	fname, err := media.HandleUpload(ctx, db, storage, nil, &feed.ID, bytes.NewReader(pngBytes))
	require.NoError(t, err)
	require.Contains(t, fname, ".png")

	exists, err := storage.ObjectExists(ctx, fname)
	require.NoError(t, err)
	require.True(t, exists)

	r, _, contentType, err := storage.DownloadFile(ctx, fname)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	require.Equal(t, "image/png", contentType)

	row, err := factory.GetMediaUploadByFname(ctx, db, fname)
	require.NoError(t, err)
	require.True(t, row.RSSFeedID.Valid)
	require.Equal(t, feed.ID, row.RSSFeedID.String)
	require.False(t, row.UserID.Valid, "a feed upload must not also be linked to a user")
}
