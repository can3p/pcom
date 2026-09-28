package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/stretchr/testify/require"

	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
)

func TestNew(t *testing.T) {
	ctx := context.Background()

	t.Run("creates server with storage", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(storage)
		require.NoError(t, err)

		// The returned server must actually be wired to the storage it was
		// given: it should be able to fetch and convert the uploaded file.
		_, mime, err := srv.GetImage(ctx, "test.png", "")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)
	})

	t.Run("applies options", func(t *testing.T) {
		image := createTestImage(t, 400, 400)
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", image, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 200, Height: 200}),
			server.WithPermaCache(true),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)

		resp := w.Result()
		require.Equal(t, "image/webp", resp.Header.Get("Content-Type"))
		require.Contains(t, resp.Header.Get("Cache-Control"), "max-age=604800", "WithPermaCache should add cache headers")

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		resized, err := vips.NewImageFromReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer resized.Close()

		// The "thumb" class should have resized the 400x400 source down to 200x200.
		require.Equal(t, 200, resized.Width())
		require.Equal(t, 200, resized.Height())
	})

	t.Run("with custom class resolver", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		resolverCalled := false
		customResolver := func(ctx context.Context, req *http.Request) string {
			resolverCalled = true
			return "custom"
		}

		srv, _, err := server.New(
			storage,
			server.WithClass("custom", server.ClassParams{Width: 50, Height: 50}),
			server.WithClassResolver(customResolver),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		// The query string names a class the resolver ignores; if the
		// resolver weren't actually consulted, this class wouldn't resolve
		// to a configured one and ServeImage would 404 instead of 200.
		req := httptest.NewRequest("GET", "/?class=unmapped", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)
		require.True(t, resolverCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}

// createTestImage creates a real JPEG-encoded image with the given exact
// dimensions (in pixels), so tests that check resize bounds have a fixture
// whose starting size is known.
func createTestImage(t *testing.T, width, height int) []byte {
	t.Helper()

	img, err := vips.Black(width, height)
	require.NoError(t, err)
	defer img.Close()

	ep := vips.NewDefaultJPEGExportParams()
	data, _, err := img.Export(ep)
	require.NoError(t, err)

	return data
}

// minimalJPEG is a minimal 1x1 pixel JPEG for testing
// This is a valid JPEG SOI marker + EOI marker with minimal content
var minimalJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
	0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
	0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
	0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
	0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
	0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
	0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
	0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
	0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
	0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
	0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
	0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD0, 0xFF, 0xD9,
}

func TestGetImage(t *testing.T) {
	ctx := context.Background()

	t.Run("downloads and processes image without class params", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(storage)
		require.NoError(t, err)

		reader, mime, err := srv.GetImage(ctx, "test.png", "")

		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)
		require.NotNil(t, reader)

		// Verify we can read the data
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Greater(t, len(data), 0)
	})

	t.Run("applies resize transformation for matching class", func(t *testing.T) {
		// Create a 200x200 test image
		largeImage := createTestImage(t, 200, 200)
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", largeImage, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		reader, mime, err := srv.GetImage(ctx, "test.png", "thumb")

		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		// Read and decode the WebP response to verify dimensions
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Greater(t, len(data), 0)

		// Decode the returned WebP to check dimensions
		resizedImg, err := vips.NewImageFromReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer resizedImg.Close()

		// The 200x200 source should be scaled down to exactly fit the
		// 100x100 bounding box.
		require.Equal(t, 100, resizedImg.Width())
		require.Equal(t, 100, resizedImg.Height())
	})

	t.Run("does not upscale images smaller than bounding box", func(t *testing.T) {
		// Create a 50x50 test image
		smallImage := createTestImage(t, 50, 50)
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "small.png", smallImage, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("large", server.ClassParams{Width: 200, Height: 200}),
		)
		require.NoError(t, err)

		reader, mime, err := srv.GetImage(ctx, "small.png", "large")

		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		// Read and decode the WebP response to verify dimensions
		data, err := io.ReadAll(reader)
		require.NoError(t, err)

		// Decode the returned WebP to check dimensions
		resizedImg, err := vips.NewImageFromReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer resizedImg.Close()

		// The 50x50 source is smaller than the 200x200 bounding box, so it
		// must be returned unchanged rather than upscaled.
		require.Equal(t, 50, resizedImg.Width())
		require.Equal(t, 50, resizedImg.Height())
	})

	t.Run("handles missing file", func(t *testing.T) {
		storage := fakestorage.New()
		srv, _, err := server.New(storage)
		require.NoError(t, err)

		_, _, err = srv.GetImage(ctx, "nonexistent.png", "")
		require.Error(t, err)
	})

	t.Run("handles storage download error", func(t *testing.T) {
		storage := fakestorage.New()
		storage.FailDownloadWith(errors.New("storage error"))

		srv, _, err := server.New(storage)
		require.NoError(t, err)

		_, _, err = srv.GetImage(ctx, "test.png", "")
		require.Error(t, err)
	})

	t.Run("handles corrupted image data", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "corrupted.png", []byte("not an image"), "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(storage)
		require.NoError(t, err)

		_, _, err = srv.GetImage(ctx, "corrupted.png", "")
		require.Error(t, err)
	})
}

func TestServeImage(t *testing.T) {
	ctx := context.Background()

	t.Run("serves image with correct headers", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)

		// Use the server as a getter
		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)

		resp := w.Result()
		defer func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		}()

		require.Equal(t, "image/webp", resp.Header.Get("Content-Type"))
		data, _ := io.ReadAll(resp.Body)
		require.Greater(t, len(data), 0)
	})

	t.Run("returns 404 for unknown class", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=unknown", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)

		resp := w.Result()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("applies cache headers when enabled", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
			server.WithPermaCache(true),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)

		cacheControl := w.Header().Get("Cache-Control")
		require.Contains(t, cacheControl, "max-age=604800")
		require.Contains(t, cacheControl, "immutable")
	})

	t.Run("handles getter error", func(t *testing.T) {
		storage := fakestorage.New()
		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)

		err = srv.ServeImage(ctx, srv, req, w, "nonexistent.png")
		require.Error(t, err)
	})

	t.Run("uses custom class resolver", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		resolverCalled := false
		customResolver := func(ctx context.Context, req *http.Request) string {
			resolverCalled = true
			return "custom"
		}

		srv, _, err := server.New(
			storage,
			server.WithClass("custom", server.ClassParams{Width: 50, Height: 50}),
			server.WithClassResolver(customResolver),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.png")
		require.NoError(t, err)
		require.True(t, resolverCalled)
	})

	t.Run("respects closer interface on reader", func(t *testing.T) {
		closeCalled := false

		// Create a mock getter that returns a closeable reader
		mockGetter := &mockGetter{
			closeCalled: &closeCalled,
		}

		storage := fakestorage.New()
		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)

		err = srv.ServeImage(ctx, mockGetter, req, w, "test.png")
		require.NoError(t, err)
		require.True(t, closeCalled, "reader should have been closed")
	})
}

type mockGetter struct {
	closeCalled *bool
}

func (m *mockGetter) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	return &closeTrackingReader{
		reader:      bytes.NewReader(minimalJPEG),
		closeCalled: m.closeCalled,
	}, "image/webp", nil
}

func (m *mockGetter) ServeImage(ctx context.Context, getter server.MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error {
	return nil
}

type closeTrackingReader struct {
	reader      io.Reader
	closeCalled *bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *closeTrackingReader) Close() error {
	*c.closeCalled = true
	return nil
}

func TestGetImage_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("large image downsizing", func(t *testing.T) {
		largeImage := createTestImage(t, 500, 500)
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "large.jpg", largeImage, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("small", server.ClassParams{Width: 10, Height: 10}),
		)
		require.NoError(t, err)

		reader, mime, err := srv.GetImage(ctx, "large.jpg", "small")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)

		resizedImg, err := vips.NewImageFromReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer resizedImg.Close()

		require.Equal(t, 10, resizedImg.Width())
		require.Equal(t, 10, resizedImg.Height())
	})

	t.Run("multiple class params", func(t *testing.T) {
		image := createTestImage(t, 1000, 1000)
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.jpg", image, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("small", server.ClassParams{Width: 50, Height: 50}),
			server.WithClass("medium", server.ClassParams{Width: 200, Height: 200}),
			server.WithClass("large", server.ClassParams{Width: 800, Height: 800}),
		)
		require.NoError(t, err)

		// Each class must resize the 1000x1000 source to its own bounds.
		classSizes := map[string]int{"small": 50, "medium": 200, "large": 800}
		for _, class := range []string{"small", "medium", "large"} {
			reader, mime, err := srv.GetImage(ctx, "test.jpg", class)
			require.NoError(t, err, "failed for class %s", class)
			require.Equal(t, "image/webp", mime)

			data, err := io.ReadAll(reader)
			require.NoError(t, err)

			resizedImg, err := vips.NewImageFromReader(bytes.NewReader(data))
			require.NoError(t, err, "failed to decode output for class %s", class)

			require.Equal(t, classSizes[class], resizedImg.Width(), "unexpected width for class %s", class)
			require.Equal(t, classSizes[class], resizedImg.Height(), "unexpected height for class %s", class)
			resizedImg.Close()
		}
	})

	t.Run("class with zero dimensions", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.jpg", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("zero", server.ClassParams{Width: 0, Height: 0}),
		)
		require.NoError(t, err)

		reader, mime, err := srv.GetImage(ctx, "test.jpg", "zero")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Greater(t, len(data), 0)
	})
}

func TestServeImage_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("empty class returns 404", func(t *testing.T) {
		storage := fakestorage.New()
		err := storage.UploadFile(ctx, "test.jpg", minimalJPEG, "image/jpeg")
		require.NoError(t, err)

		srv, _, err := server.New(
			storage,
			server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=", nil)

		err = srv.ServeImage(ctx, srv, req, w, "test.jpg")
		require.NoError(t, err)

		resp := w.Result()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

}
