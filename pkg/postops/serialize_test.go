package postops_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSerializePost_Golden(t *testing.T) {
	t.Parallel()

	full := &model.Post{
		ID:               "018f45ef-b63a-7426-a444-3957146ca700",
		Subject:          new("A test post"),
		Body:             "Some *markdown* body.\n\nWith a second paragraph.",
		VisibilityRadius: model.PostVisibilitySecondDegree,
		PublishedAt:      new(time.Date(2025, time.January, 3, 1, 46, 49, 0, time.UTC)),
		URLID:            new("url-id"),
	}
	full.URL = &model.NormalizedURL{URL: "https://example.com/article"}

	golden.Assert(t, "serialize_post_full", postops.SerializePost(full))

	minimal := &model.Post{
		ID:               "018f45ef-b63a-7426-a444-3957146ca701",
		Body:             "Just a body, nothing else.",
		VisibilityRadius: model.PostVisibilityDirectOnly,
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

	post := &model.Post{
		ID:               uuid.NewString(),
		Body:             fmt.Sprintf("See this photo: ![alt](%s)", imgName),
		VisibilityRadius: model.PostVisibilityPublic,
	}

	b, err := postops.SerializeBlogSlice(context.Background(), []*model.Post{post}, storage)
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

	post := &model.Post{
		ID:               uuid.NewString(),
		Body:             fmt.Sprintf("![alt](%s)", imgName),
		VisibilityRadius: model.PostVisibilityPublic,
	}

	b, err := postops.SerializeBlogSlice(context.Background(), []*model.Post{post}, storage)
	require.NoError(t, err)

	files := zipFileNames(t, b)
	require.Contains(t, files, imgName)
	require.NotContains(t, files, "missing_images.txt")
	require.Equal(t, pngBytes, zipFileContentBytes(t, b, imgName))
}

func TestSerializeBlogSlice_ExternalImageIsNeverDownloaded(t *testing.T) {
	t.Parallel()

	post := &model.Post{
		ID:               uuid.NewString(),
		Body:             "![alt](https://example.com/pic.png)",
		VisibilityRadius: model.PostVisibilityPublic,
	}

	// nil storage: if isURLMediaUpload correctly filters out the external
	// URL, DownloadFile is never called and a nil storage is safe to pass.
	b, err := postops.SerializeBlogSlice(context.Background(), []*model.Post{post}, nil)
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

	post := &model.Post{
		ID:               uuid.NewString(),
		Body:             "![alt](3fa85f64-5717-4562-b3fc-2c963f66afa6.png)",
		VisibilityRadius: model.PostVisibilityPublic,
	}

	_, err := postops.SerializeBlogSlice(context.Background(), []*model.Post{post}, storage)
	require.ErrorIs(t, err, boom)
}

func TestSerializeBlogSlice_ClosesArchiveOnce(t *testing.T) {
	t.Parallel()

	post := &model.Post{
		ID:               uuid.NewString(),
		Body:             "no images here",
		VisibilityRadius: model.PostVisibilityPublic,
	}

	b, err := postops.SerializeBlogSlice(context.Background(), []*model.Post{post}, nil)
	require.NoError(t, err)

	// a finished archive parses and lists the post
	require.Equal(t, []string{post.ID + ".md"}, zipFileNames(t, b))
}

func TestSerializeBlogSlice_SharedImageIsWrittenOnce(t *testing.T) {
	t.Parallel()

	storage := fakestorage.New()
	present := "3fa85f64-5717-4562-b3fc-2c963f66afa6.png"
	missing := "4fa85f64-5717-4562-b3fc-2c963f66afa6.png"
	require.NoError(t, storage.UploadFile(context.Background(), present, []byte("png"), "image/png"))

	posts := make([]*model.Post, 2)
	for i := range posts {
		posts[i] = &model.Post{
			ID:               uuid.NewString(),
			Body:             fmt.Sprintf("![a](%s) ![b](%s)", present, missing),
			VisibilityRadius: model.PostVisibilityPublic,
		}
	}

	b, err := postops.SerializeBlogSlice(context.Background(), posts, storage)
	require.NoError(t, err)

	files := zipFileNames(t, b)
	seen := map[string]bool{}
	for _, f := range files {
		require.False(t, seen[f], "duplicate zip entry %s", f)
		seen[f] = true
	}
	require.Contains(t, files, present)
	require.Equal(t, missing, string(zipFileContentBytes(t, b, "missing_images.txt")))
}
