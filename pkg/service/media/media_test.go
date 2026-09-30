package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	mediapkg "github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

// TestUpload covers success and error paths for uploading media.
func TestUpload(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)

	cases := []struct {
		name         string
		actor        *core.User
		data         []byte
		wantErr      bool
		checkStorage bool
		checkRow     func(*testing.T, *core.MediaUpload)
	}{
		{
			name:         "success",
			actor:        user,
			data:         pngBytes,
			wantErr:      false,
			checkStorage: true,
			checkRow: func(t *testing.T, row *core.MediaUpload) {
				require.True(t, row.UserID.Valid)
				require.Equal(t, user.ID, row.UserID.String)
				require.False(t, row.RSSFeedID.Valid, "a user upload must not also be linked to a feed")
			},
		},
		{
			name:    "nil actor returns ErrNeedsLogin",
			actor:   nil,
			data:    pngBytes,
			wantErr: true,
		},
		{
			name:    "unsupported MIME type",
			actor:   user,
			data:    []byte("text data, not an image"),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage := fakestorage.New()
			svc := media.New(store, storage)

			fname, err := svc.Upload(ctx, tc.actor, bytes.NewReader(tc.data))

			if tc.wantErr {
				require.Error(t, err)
				if tc.actor == nil {
					require.ErrorIs(t, err, service.ErrNeedsLogin)
				}
				return
			}

			require.NoError(t, err)
			require.Contains(t, fname, ".png")

			if tc.checkStorage {
				exists := testutil.Must(storage.ObjectExists(ctx, fname))(t)
				require.True(t, exists, "the stored object should exist")

				r, _, contentType, err := storage.DownloadFile(ctx, fname)
				require.NoError(t, err)
				defer func() { _ = r.Close() }()
				require.Equal(t, "image/png", contentType)
			}

			if tc.checkRow != nil {
				row := testutil.Must(factory.GetMediaUploadByFname(ctx, db, fname))(t)
				tc.checkRow(t, row)
			}
		})
	}
}

var (
	errRead    = errors.New("connection reset")
	errStorage = errors.New("storage unavailable")
)

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errRead }

// TestUpload_Errors checks that each failure comes back as itself, so a
// swallowed error can't pass as a different rejection.
func TestUpload_Errors(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)

	cases := []struct {
		name    string
		reader  io.Reader
		storage func(*fakestorage.Storage)
		wantErr error
	}{
		{"failing reader", failReader{}, nil, errRead},
		{"unsupported MIME type", bytes.NewReader([]byte("text data, not an image")), nil, mediapkg.ErrUnsupportedMimeType},
		{"failing storage", bytes.NewReader(pngBytes), func(s *fakestorage.Storage) { s.FailUploadWith(errStorage) }, errStorage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage := fakestorage.New()
			if tc.storage != nil {
				tc.storage(storage)
			}

			_, err := media.New(store, storage).Upload(ctx, user, tc.reader)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
