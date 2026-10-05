package media_test

import (
	"bytes"
	"context"
	"testing"

	mediapkg "github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestStoreUpload_ArgumentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userID    *string
		rssFeedID *string
		desc      string
	}{
		{
			name:      "both_nil",
			userID:    nil,
			rssFeedID: nil,
			desc:      "both userID and rssFeedID are nil",
		},
		{
			name:      "both_provided",
			userID:    new("user123"),
			rssFeedID: new("feed456"),
			desc:      "both userID and rssFeedID are provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The error is returned before the store is used, so nil is a
			// sentinel for that early return.
			mockStorage := fakestorage.New()

			pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
			reader := bytes.NewReader(pngData)

			_, err := media.StoreUpload(context.Background(), nil, mockStorage, tt.userID, tt.rssFeedID, reader)

			require.Error(t, err, "expected error when "+tt.desc)
			require.Contains(t, err.Error(), "exactly one of")
		})
	}
}

func TestStoreUpload_ReaderError(t *testing.T) {
	t.Parallel()

	_, err := media.StoreUpload(context.Background(), nil, fakestorage.New(), new("user123"), nil, failReader{})
	require.Error(t, err)
}

func TestStoreUpload_InvalidImageType(t *testing.T) {
	t.Parallel()

	userID := new("user123")
	mockStorage := fakestorage.New()

	// Create a reader with data that detects as text, not an image
	textData := []byte("This is plain text, not an image")
	reader := bytes.NewReader(textData)

	_, err := media.StoreUpload(context.Background(), nil, mockStorage, userID, nil, reader)

	// Should fail because text is not a supported image type
	require.Error(t, err)
	require.ErrorIs(t, err, mediapkg.ErrUnsupportedMimeType)
}

// TestStoreUpload_StoresRowAndObject checks both halves of a successful
// upload, for each of the two things it can be linked to: the object lands
// in storage under the returned name, and the media_uploads row is linked to
// that owner only, never both.
func TestStoreUpload_StoresRowAndObject(t *testing.T) {
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

			fname := testutil.Must(media.StoreUpload(ctx, repo.New(db), storage, userID, feedID, bytes.NewReader(pngBytes)))(t)
			require.Contains(t, fname, ".png")

			exists := testutil.Must(storage.ObjectExists(ctx, fname))(t)
			require.True(t, exists, "the stored object should exist")

			r, _, contentType, err := storage.DownloadFile(ctx, fname)
			require.NoError(t, err)
			defer func() { _ = r.Close() }()
			require.Equal(t, "image/png", contentType)

			row := testutil.Must(factory.GetMediaUploadByFname(ctx, db, fname))(t)
			if userID != nil {
				require.NotNil(t, row.UserID)
				require.Equal(t, *userID, lo.FromPtr(row.UserID))
				require.Nil(t, row.RSSFeedID, "a user upload must not also be linked to a feed")
			} else {
				require.NotNil(t, row.RSSFeedID)
				require.Equal(t, *feedID, lo.FromPtr(row.RSSFeedID))
				require.Nil(t, row.UserID, "a feed upload must not also be linked to a user")
			}
		})
	}
}
