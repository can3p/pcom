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
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
)

// newServer builds a server.Server over storage, failing the test on error.
func newServer(t *testing.T, storage server.MediaStorage, opts ...server.Option) *server.Server {
	t.Helper()

	srv, _, err := server.New(storage, opts...)
	require.NoError(t, err)

	return srv
}

// createTestImage creates a real JPEG-encoded image with the given exact
// dimensions (in pixels), so tests that check resize bounds have a fixture
// whose starting size is known.
func createTestImage(t *testing.T, width, height int) []byte {
	t.Helper()

	img := testutil.Must(vips.Black(width, height))(t)
	defer img.Close()

	ep := vips.NewDefaultJPEGExportParams()
	data, _, err := img.Export(ep)
	require.NoError(t, err)

	return data
}

// minimalJPEG is a minimal 1x1 pixel JPEG for testing: a valid JPEG SOI
// marker + EOI marker with minimal content.
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

func TestNew(t *testing.T) {
	ctx := context.Background()

	t.Run("wires the given storage", func(t *testing.T) {
		storage := fakestorage.New()
		require.NoError(t, storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg"))
		srv := newServer(t, storage)

		_, mime, err := srv.GetImage(ctx, "test.png", "")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)
	})

	t.Run("applies options", func(t *testing.T) {
		image := createTestImage(t, 400, 400)
		storage := fakestorage.New()
		require.NoError(t, storage.UploadFile(ctx, "test.png", image, "image/jpeg"))
		srv := newServer(t, storage,
			server.WithClass("thumb", server.ClassParams{Width: 200, Height: 200}),
			server.WithPermaCache(true))

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)
		require.NoError(t, srv.ServeImage(ctx, srv, req, w, "test.png"))

		resp := w.Result()
		require.Equal(t, "image/webp", resp.Header.Get("Content-Type"))
		require.Contains(t, resp.Header.Get("Cache-Control"), "max-age=604800", "WithPermaCache should add cache headers")

		data := testutil.Must(io.ReadAll(resp.Body))(t)
		resized := testutil.Must(vips.NewImageFromReader(bytes.NewReader(data)))(t)
		defer resized.Close()

		// The "thumb" class should have resized the 400x400 source down to 200x200.
		require.Equal(t, 200, resized.Width())
		require.Equal(t, 200, resized.Height())
	})

	t.Run("consults a custom class resolver", func(t *testing.T) {
		storage := fakestorage.New()
		require.NoError(t, storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg"))

		resolverCalled := false
		srv := newServer(t, storage,
			server.WithClass("custom", server.ClassParams{Width: 50, Height: 50}),
			server.WithClassResolver(func(ctx context.Context, req *http.Request) string {
				resolverCalled = true
				return "custom"
			}))

		w := httptest.NewRecorder()
		// The query string names a class the resolver ignores; if the
		// resolver weren't actually consulted, this class wouldn't resolve
		// to a configured one and ServeImage would 404 instead of 200.
		req := httptest.NewRequest("GET", "/?class=unmapped", nil)
		require.NoError(t, srv.ServeImage(ctx, srv, req, w, "test.png"))

		require.True(t, resolverCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}

func TestGetImage_Resize(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name         string
		srcW, srcH   int
		classes      map[string]server.ClassParams
		requestClass string
		wantW, wantH int
	}{
		{"no class configured returns the source size", 300, 150, nil, "", 300, 150},
		{"resizes down to fit the bounding box", 200, 200,
			map[string]server.ClassParams{"thumb": {Width: 100, Height: 100}}, "thumb", 100, 100},
		{"does not upscale an image smaller than the bounding box", 50, 50,
			map[string]server.ClassParams{"large": {Width: 200, Height: 200}}, "large", 50, 50},
		{"downsizes a large image", 500, 500,
			map[string]server.ClassParams{"small": {Width: 10, Height: 10}}, "small", 10, 10},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			image := createTestImage(t, c.srcW, c.srcH)
			storage := fakestorage.New()
			require.NoError(t, storage.UploadFile(ctx, "test.jpg", image, "image/jpeg"))

			var opts []server.Option
			for name, params := range c.classes {
				opts = append(opts, server.WithClass(name, params))
			}
			srv := newServer(t, storage, opts...)

			reader, mime, err := srv.GetImage(ctx, "test.jpg", c.requestClass)
			require.NoError(t, err)
			require.Equal(t, "image/webp", mime)

			data := testutil.Must(io.ReadAll(reader))(t)
			resized := testutil.Must(vips.NewImageFromReader(bytes.NewReader(data)))(t)
			defer resized.Close()

			require.Equal(t, c.wantW, resized.Width())
			require.Equal(t, c.wantH, resized.Height())
		})
	}
}

func TestGetImage_ZeroDimensionClass(t *testing.T) {
	// A degenerate 0x0 class must not crash GetImage.
	ctx := context.Background()
	storage := fakestorage.New()
	require.NoError(t, storage.UploadFile(ctx, "test.jpg", minimalJPEG, "image/jpeg"))
	srv := newServer(t, storage, server.WithClass("zero", server.ClassParams{Width: 0, Height: 0}))

	reader, mime, err := srv.GetImage(ctx, "test.jpg", "zero")
	require.NoError(t, err)
	require.Equal(t, "image/webp", mime)

	data := testutil.Must(io.ReadAll(reader))(t)
	require.Greater(t, len(data), 0)
}

func TestGetImage_Errors(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		setup func(t *testing.T) (storage *fakestorage.Storage, fname string)
	}{
		{"missing file", func(t *testing.T) (*fakestorage.Storage, string) {
			return fakestorage.New(), "nonexistent.png"
		}},
		{"storage download error", func(t *testing.T) (*fakestorage.Storage, string) {
			storage := fakestorage.New()
			storage.FailDownloadWith(errors.New("storage error"))
			return storage, "test.png"
		}},
		{"corrupted image data", func(t *testing.T) (*fakestorage.Storage, string) {
			storage := fakestorage.New()
			require.NoError(t, storage.UploadFile(ctx, "corrupted.png", []byte("not an image"), "image/jpeg"))
			return storage, "corrupted.png"
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			storage, fname := c.setup(t)
			srv := newServer(t, storage)

			_, _, err := srv.GetImage(ctx, fname, "")
			require.Error(t, err)
		})
	}
}

// mockGetter is a MediaGetter whose returned reader tracks whether it was closed.
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

func TestServeImage(t *testing.T) {
	ctx := context.Background()

	t.Run("serves image with correct headers", func(t *testing.T) {
		storage := fakestorage.New()
		require.NoError(t, storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg"))
		srv := newServer(t, storage, server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)
		require.NoError(t, srv.ServeImage(ctx, srv, req, w, "test.png"))

		resp := w.Result()
		defer func() { require.NoError(t, resp.Body.Close()) }()

		require.Equal(t, "image/webp", resp.Header.Get("Content-Type"))
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Greater(t, len(data), 0)
	})

	t.Run("404 when the class doesn't resolve to a configured one", func(t *testing.T) {
		storage := fakestorage.New()
		require.NoError(t, storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg"))
		srv := newServer(t, storage, server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}))

		for _, class := range []string{"unknown", ""} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/?class="+class, nil)
			require.NoError(t, srv.ServeImage(ctx, srv, req, w, "test.png"))
			require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		}
	})

	t.Run("cache headers", func(t *testing.T) {
		cases := []struct {
			name         string
			permaCache   bool
			wantContains []string
		}{
			{"absent by default", false, nil},
			{"present when WithPermaCache is set", true, []string{"max-age=604800", "immutable"}},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				storage := fakestorage.New()
				require.NoError(t, storage.UploadFile(ctx, "test.png", minimalJPEG, "image/jpeg"))

				opts := []server.Option{server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100})}
				if c.permaCache {
					opts = append(opts, server.WithPermaCache(true))
				}
				srv := newServer(t, storage, opts...)

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/?class=thumb", nil)
				require.NoError(t, srv.ServeImage(ctx, srv, req, w, "test.png"))

				cacheControl := w.Header().Get("Cache-Control")
				if len(c.wantContains) == 0 {
					require.Empty(t, cacheControl)
				}
				for _, want := range c.wantContains {
					require.Contains(t, cacheControl, want)
				}
			})
		}
	})

	t.Run("propagates the getter's error", func(t *testing.T) {
		storage := fakestorage.New()
		srv := newServer(t, storage, server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)
		require.Error(t, srv.ServeImage(ctx, srv, req, w, "nonexistent.png"))
	})

	t.Run("closes the reader when it implements io.Closer", func(t *testing.T) {
		closeCalled := false
		mockGetter := &mockGetter{closeCalled: &closeCalled}

		storage := fakestorage.New()
		srv := newServer(t, storage, server.WithClass("thumb", server.ClassParams{Width: 100, Height: 100}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?class=thumb", nil)
		require.NoError(t, srv.ServeImage(ctx, mockGetter, req, w, "test.png"))
		require.True(t, closeCalled, "reader should have been closed")
	})
}
