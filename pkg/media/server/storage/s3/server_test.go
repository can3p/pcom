package s3_test

import (
	"context"
	"io"
	"os"
	"testing"

	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/media/server/storage/s3"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	_ = tommy.Cleanup()
	os.Exit(code)
}

func newStorage(t *testing.T) (*tommy.Tommy, server.MediaStorage) {
	t.Helper()

	if testing.Short() {
		t.Skip("starts a container")
	}

	tm := tommy.Shared(t)
	st, err := s3.New(s3.Options{Endpoint: tm.S3URL, Bucket: tommy.Bucket, Region: "us-east-1", Key: "k", Secret: "s", PathStyle: true})
	require.NoError(t, err)

	return tm, st
}

func TestUploadDownload(t *testing.T) {
	tm, st := newStorage(t)
	ctx := context.Background()
	key := "s3test/round-trip.png"
	body := []byte("not really a png")

	require.NoError(t, st.UploadFile(ctx, key, body, "image/png"))

	r, size, contentType, err := st.DownloadFile(ctx, key)
	require.NoError(t, err)

	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, body, got)
	require.Equal(t, int64(len(body)), size)
	require.Equal(t, "image/png", contentType)

	obj := tm.S3Object(t, key)
	require.Equal(t, "image/png", obj.ContentType)
	require.Equal(t, int64(len(body)), obj.Size)

	var put *tommy.S3Event

	for _, e := range tm.S3Events(t, "s3.object.put") {
		if e.Key == key {
			put = &e
		}
	}

	require.NotNil(t, put, "no put event for %s", key)
	require.Equal(t, []string{"private"}, put.Headers["X-Amz-Acl"])
}

func TestObjectExists(t *testing.T) {
	_, st := newStorage(t)
	ctx := context.Background()

	require.NoError(t, st.UploadFile(ctx, "s3test/exists.txt", []byte("x"), "text/plain"))

	for key, want := range map[string]bool{"s3test/exists.txt": true, "s3test/nope.txt": false} {
		ok, err := st.ObjectExists(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, want, ok, key)
	}
}

func TestDownloadMissing(t *testing.T) {
	_, st := newStorage(t)

	_, _, _, err := st.DownloadFile(context.Background(), "s3test/missing.png")
	require.ErrorIs(t, err, mediaerrors.ErrNotFound)
}
