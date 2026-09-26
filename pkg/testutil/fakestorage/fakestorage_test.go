package fakestorage_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/stretchr/testify/require"
)

func TestStorage_RoundTrips(t *testing.T) {
	t.Parallel()

	s := fakestorage.New()
	ctx := context.Background()

	require.NoError(t, s.UploadFile(ctx, "a.png", []byte("hello"), "image/png"))

	exists, err := s.ObjectExists(ctx, "a.png")
	require.NoError(t, err)
	require.True(t, exists)

	r, size, contentType, err := s.DownloadFile(ctx, "a.png")
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
	require.EqualValues(t, len("hello"), size)
	require.Equal(t, "image/png", contentType)
}

func TestStorage_MissingObject(t *testing.T) {
	t.Parallel()

	s := fakestorage.New()
	ctx := context.Background()

	exists, err := s.ObjectExists(ctx, "missing.png")
	require.NoError(t, err)
	require.False(t, exists)

	_, _, _, err = s.DownloadFile(ctx, "missing.png")
	require.ErrorIs(t, err, media.ErrNotFound)
}

func TestStorage_InjectedErrors(t *testing.T) {
	t.Parallel()

	s := fakestorage.New()
	ctx := context.Background()
	boom := errors.New("boom")

	s.FailUploadWith(boom)
	require.ErrorIs(t, s.UploadFile(ctx, "a.png", nil, ""), boom)
	s.FailUploadWith(nil)
	require.NoError(t, s.UploadFile(ctx, "a.png", []byte("x"), "text/plain"))

	s.FailDownloadWith(boom)
	_, _, _, err := s.DownloadFile(ctx, "a.png")
	require.ErrorIs(t, err, boom)
	s.FailDownloadWith(nil)

	s.FailExistsWith(boom)
	_, err = s.ObjectExists(ctx, "a.png")
	require.ErrorIs(t, err, boom)
	s.FailExistsWith(nil)

	exists, err := s.ObjectExists(ctx, "a.png")
	require.NoError(t, err)
	require.True(t, exists, "clearing the injected error should reveal the file uploaded earlier")
}
