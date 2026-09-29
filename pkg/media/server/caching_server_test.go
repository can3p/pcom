package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/stretchr/testify/require"
)

type mockStorage struct {
	mu        sync.RWMutex
	files     map[string][]byte
	existsErr error
	uploadErr error
	callCount struct {
		exists   int
		download int
		upload   int
	}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		files: make(map[string][]byte),
	}
}

func (m *mockStorage) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount.upload++
	if m.uploadErr != nil {
		return m.uploadErr
	}
	m.files[fname] = b
	return nil
}

func (m *mockStorage) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	// A plain Lock, not RLock: this mutates callCount.download, so
	// concurrent calls (as exercised by the concurrency tests below) must
	// not race on that counter.
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount.download++
	if data, ok := m.files[fname]; ok {
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), "image/webp", nil
	}
	return nil, 0, "", mediaerrors.ErrNotFound
}

func (m *mockStorage) ObjectExists(ctx context.Context, fname string) (bool, error) {
	// A plain Lock, not RLock: see DownloadFile above.
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount.exists++
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, exists := m.files[fname]
	return exists, nil
}

// uploadCount reads callCount.upload under the lock, so it's safe to poll
// from a require.Eventually callback while an upload may still be in flight
// on another goroutine.
func (m *mockStorage) uploadCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount.upload
}

type mockServer struct {
	callCount int
	mu        sync.Mutex
}

func (m *mockServer) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return bytes.NewReader([]byte("test image")), "image/webp", nil
}

func (m *mockServer) ServeImage(ctx context.Context, getter MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error {
	return nil
}

type errMockServer struct {
	err error
}

func (e *errMockServer) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	return nil, "", e.err
}

func (e *errMockServer) ServeImage(ctx context.Context, getter MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error {
	return e.err
}

func TestNewCachingServer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		size         int
		wantCapacity int
	}{
		{"negative cache size uses default", -1, defaultCacheSize},
		{"zero cache size uses default", 0, defaultCacheSize},
		{"positive cache size is respected", 50, 50},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cache := testutil.Must(NewCachingServer(&mockServer{}, newMockStorage(), c.size))(t)

			for i := range c.wantCapacity + 1 {
				cache.cache.Add(fmt.Sprintf("key-%d", i), true)
			}
			require.Equal(t, c.wantCapacity, cache.cache.Len(), "cache should be capped at the configured size")
		})
	}
}

func TestCachingServer_GetImage(t *testing.T) {
	ctx := context.Background()
	t.Run("first request fetches from parent and caches, subsequent uses cache", func(t *testing.T) {
		storage := newMockStorage()
		srv := &mockServer{}
		cache := testutil.Must(NewCachingServer(srv, storage, 10))(t)

		reader, mime, err := cache.GetImage(ctx, "test.jpg", "thumb")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		data := testutil.Must(io.ReadAll(reader))(t)
		require.Equal(t, "test image", string(data))

		require.Eventually(t, func() bool {
			return storage.uploadCount() == 1
		}, 5*time.Second, 10*time.Millisecond, "Expected 1 upload after cache")

		require.Equal(t, 1, srv.callCount, "Expected 1 parent server call")

		addCalls := 3
		for range addCalls {
			reader2, _, err2 := cache.GetImage(ctx, "test.jpg", "thumb")
			require.NoError(t, err2)
			data2 := testutil.Must(io.ReadAll(reader2))(t)
			require.Equal(t, "test image", string(data2))
		}
		require.Equal(t, 1, srv.callCount, "Expected parent server call count to remain 1 after subsequent requests")
		require.Equal(t, addCalls, storage.callCount.download, "Expected storage to be hit exactly the number of additional calls")
	})

	t.Run("error when checking storage existence", func(t *testing.T) {
		storage := newMockStorage()
		storage.existsErr = errors.New("existence check failed")
		srv := &mockServer{}
		cache := testutil.Must(NewCachingServer(srv, storage, 10))(t)

		// ObjectExists fails on both checks, but GetImage must still fall
		// through to the parent server rather than error out.
		reader, mime, err := cache.GetImage(ctx, "missing.jpg", "thumb")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)
		data := testutil.Must(io.ReadAll(reader))(t)
		require.Equal(t, "test image", string(data))
		require.Equal(t, 1, srv.callCount, "expected fallback to the parent server")
	})

	t.Run("parent server error propagates", func(t *testing.T) {
		storage := newMockStorage()
		errServer := &errMockServer{err: errors.New("server error")}
		cache := testutil.Must(NewCachingServer(errServer, storage, 10))(t)

		_, _, err := cache.GetImage(ctx, "test.jpg", "thumb")
		require.Error(t, err)
	})

	t.Run("upload failure doesn't break response to client", func(t *testing.T) {
		storage := newMockStorage()
		storage.uploadErr = errors.New("upload failed")
		srv := &mockServer{}
		cache := testutil.Must(NewCachingServer(srv, storage, 10))(t)

		reader, mime, err := cache.GetImage(ctx, "test.jpg", "thumb")
		require.NoError(t, err)
		require.Equal(t, "image/webp", mime)

		data := testutil.Must(io.ReadAll(reader))(t)
		require.Equal(t, "test image", string(data))

		require.Eventually(t, func() bool {
			return storage.uploadCount() == 1
		}, 5*time.Second, 10*time.Millisecond, "expected an upload attempt despite the injected failure")

		require.False(t, cache.cache.Contains(cache.getCacheKey("test.jpg", "thumb")), "a failed upload must not be recorded as cached")
	})

	t.Run("concurrent requests for same image", func(t *testing.T) {
		storage := newMockStorage()
		srv := &mockServer{}
		cache := testutil.Must(NewCachingServer(srv, storage, 10))(t)

		var wg sync.WaitGroup
		for range 5 {
			wg.Go(func() {
				reader, _, err := cache.GetImage(ctx, "new.jpg", "thumb")
				if err != nil {
					t.Errorf("Failed to get image: %v", err)
					return
				}
				_, _ = io.ReadAll(reader) // Read to close
			})
		}
		wg.Wait()

		// Should only call parent server once
		require.Equal(t, 1, srv.callCount, "Expected 1 parent server call (from concurrent requests)")
	})
}
