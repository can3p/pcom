package media_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
)

var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

// TestHandleUpload_StoresRowAndObject checks both halves of a successful
// upload, for each of the two things it can be linked to: the object lands
// in storage under the returned name, and the media_uploads row is linked to
// that owner only, never both.
func TestHandleUpload_StoresRowAndObject(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	cases := []struct {
		name     string
		newOwner func(t *testing.T) (userID, feedID *string)
	}{
		{"for a user", func(t *testing.T) (*string, *string) {
			owner := testutil.Must(factory.User(ctx, db))(t)
			return &owner.ID, nil
		}},
		{"for a feed", func(t *testing.T) (*string, *string) {
			feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
			return nil, &feed.ID
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage := fakestorage.New()
			userID, feedID := tc.newOwner(t)

			fname := testutil.Must(media.HandleUpload(ctx, db, storage, userID, feedID, bytes.NewReader(pngBytes)))(t)
			require.Contains(t, fname, ".png")

			exists := testutil.Must(storage.ObjectExists(ctx, fname))(t)
			require.True(t, exists, "the stored object should exist")

			r, _, contentType, err := storage.DownloadFile(ctx, fname)
			require.NoError(t, err)
			defer func() { _ = r.Close() }()
			require.Equal(t, "image/png", contentType)

			row := testutil.Must(factory.GetMediaUploadByFname(ctx, db, fname))(t)
			if userID != nil {
				require.True(t, row.UserID.Valid)
				require.Equal(t, *userID, row.UserID.String)
				require.False(t, row.RSSFeedID.Valid, "a user upload must not also be linked to a feed")
			} else {
				require.True(t, row.RSSFeedID.Valid)
				require.Equal(t, *feedID, row.RSSFeedID.String)
				require.False(t, row.UserID.Valid, "a feed upload must not also be linked to a user")
			}
		})
	}
}
