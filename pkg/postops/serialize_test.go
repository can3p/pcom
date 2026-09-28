package postops_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func TestSerializePost_Golden(t *testing.T) {
	t.Parallel()

	full := &core.Post{
		ID:               "018f45ef-b63a-7426-a444-3957146ca700",
		Subject:          null.StringFrom("A test post"),
		Body:             "Some *markdown* body.\n\nWith a second paragraph.",
		VisibilityRadius: core.PostVisibilitySecondDegree,
		PublishedAt:      null.TimeFrom(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
		URLID:            null.StringFrom("url-id"),
	}
	full.R = full.R.NewStruct()
	full.R.URL = &core.NormalizedURL{URL: "https://example.com/article"}

	golden.Assert(t, "serialize_post_full", postops.SerializePost(full))

	minimal := &core.Post{
		ID:               "018f45ef-b63a-7426-a444-3957146ca701",
		Body:             "Just a body, nothing else.",
		VisibilityRadius: core.PostVisibilityDirectOnly,
	}

	golden.Assert(t, "serialize_post_minimal", postops.SerializePost(minimal))
}

// zipFileNames and zipFileContentBytes read back what SerializeBlogSlice
// produced, the same way any zip reader would.
func zipFileNames(t *testing.T, b []byte) []string {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}

	return names
}

func zipFileContentBytes(t *testing.T, b []byte, name string) []byte {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)

	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			defer func() { _ = rc.Close() }()

			content, err := io.ReadAll(rc)
			require.NoError(t, err)

			return content
		}
	}

	t.Fatalf("zip file %q not found", name)
	return nil
}

func TestSerializeBlogSlice_MissingImageIsListedNotFailed(t *testing.T) {
	t.Parallel()

	storage := fakestorage.New()
	imgName := "3fa85f64-5717-4562-b3fc-2c963f66afa6.png"

	post := &core.Post{
		ID:               uuid.NewString(),
		Body:             fmt.Sprintf("See this photo: ![alt](%s)", imgName),
		VisibilityRadius: core.PostVisibilityPublic,
	}

	b, err := postops.SerializeBlogSlice(context.Background(), []*core.Post{post}, storage)
	require.NoError(t, err)

	files := zipFileNames(t, b)
	require.Contains(t, files, "missing_images.txt")
	require.NotContains(t, files, imgName)

	content := zipFileContentBytes(t, b, "missing_images.txt")
	require.Equal(t, imgName, string(content))
}

func TestSerializeBlogSlice_ExistingImageIsDownloadedAndIncluded(t *testing.T) {
	t.Parallel()

	storage := fakestorage.New()
	imgName := "3fa85f64-5717-4562-b3fc-2c963f66afa6.png"
	pngBytes := []byte("\x89PNG\r\n\x1a\nrest-of-file")
	require.NoError(t, storage.UploadFile(context.Background(), imgName, pngBytes, "image/png"))

	post := &core.Post{
		ID:               uuid.NewString(),
		Body:             fmt.Sprintf("![alt](%s)", imgName),
		VisibilityRadius: core.PostVisibilityPublic,
	}

	b, err := postops.SerializeBlogSlice(context.Background(), []*core.Post{post}, storage)
	require.NoError(t, err)

	files := zipFileNames(t, b)
	require.Contains(t, files, imgName)
	require.NotContains(t, files, "missing_images.txt")
	require.Equal(t, pngBytes, zipFileContentBytes(t, b, imgName))
}

func TestSerializeBlogSlice_ExternalImageIsNeverDownloaded(t *testing.T) {
	t.Parallel()

	post := &core.Post{
		ID:               uuid.NewString(),
		Body:             "![alt](https://example.com/pic.png)",
		VisibilityRadius: core.PostVisibilityPublic,
	}

	// nil storage: if isURLMediaUpload correctly filters out the external
	// URL, DownloadFile is never called and a nil storage is safe to pass.
	b, err := postops.SerializeBlogSlice(context.Background(), []*core.Post{post}, nil)
	require.NoError(t, err)

	files := zipFileNames(t, b)
	require.NotContains(t, files, "missing_images.txt")
	require.Len(t, files, 1)
}

func TestSerializeBlogSlice_DownloadErrorPropagates(t *testing.T) {
	t.Parallel()

	storage := fakestorage.New()
	boom := errors.New("boom")
	storage.FailDownloadWith(boom)

	post := &core.Post{
		ID:               uuid.NewString(),
		Body:             "![alt](3fa85f64-5717-4562-b3fc-2c963f66afa6.png)",
		VisibilityRadius: core.PostVisibilityPublic,
	}

	_, err := postops.SerializeBlogSlice(context.Background(), []*core.Post{post}, storage)
	require.ErrorIs(t, err, boom)
}

// Not parallel: it redirects the shared standard logger for the duration
// of the call.
func TestSerializeBlogSlice_ClosesArchiveOnce(t *testing.T) {
	t.Skip("known bug #110: SerializeBlogSlice closes the zip writer twice and logs \"zip: writer closed twice\"")

	post := &core.Post{
		ID:               uuid.NewString(),
		Body:             "no images here",
		VisibilityRadius: core.PostVisibilityPublic,
	}

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	b, err := postops.SerializeBlogSlice(context.Background(), []*core.Post{post}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, b)

	require.NotContains(t, buf.String(), "zip: writer closed twice")
}
