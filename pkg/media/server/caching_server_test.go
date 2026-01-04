package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/media/errors"
	"github.com/stretchr/testify/require"
)

type mockStorage struct {
	mu        sync.RWMutex
	files     map[string][]byte
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
	m.files[fname] = b
	return nil
}

func (m *mockStorage) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.callCount.download++
	if data, ok := m.files[fname]; ok {
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), "image/webp", nil
	}
	return nil, 0, "", errors.ErrNotFound
}

func (m *mockStorage) ObjectExists(ctx context.Context, fname string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.callCount.exists++
	_, exists := m.files[fname]
	return exists, nil
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

func TestCachingServer_GetImage(t *testing.T) {
	ctx := context.Background()
	t.Run("first request fetches from parent and caches, subsequent uses cache", func(t *testing.T) {
		storage := newMockStorage()
		server := &mockServer{}
		cache, err := NewCachingServer(server, storage, 10)
		if err != nil {
			t.Fatalf("Failed to create caching server: %v", err)
		}

		reader, mime, err := cache.GetImage(ctx, "test.jpg", "thumb")
		if err != nil {
			t.Fatalf("Failed to get image: %v", err)
		}
		if mime != "image/webp" {
			t.Errorf("Expected mime type image/webp, got %s", mime)
		}

		// Read the image
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, "test image", string(data))

		require.Eventually(t, func() bool {
			return storage.callCount.upload == 1
		}, 5*time.Second, 10*time.Millisecond, "Expected 1 upload after cache")

		require.Equal(t, 1, server.callCount, "Expected 1 parent server call")

		addCalls := 3
		for range addCalls {
			reader2, _, err2 := cache.GetImage(ctx, "test.jpg", "thumb")
			require.NoError(t, err2)
			data2, err2 := io.ReadAll(reader2)
			require.NoError(t, err2)
			require.Equal(t, "test image", string(data2))
		}
		require.Equal(t, 1, server.callCount, "Expected parent server call count to remain 1 after subsequent requests")
		require.Equal(t, addCalls, storage.callCount.download, "Expected storage to be hit exactly the number of additional calls")
	})

	t.Run("concurrent requests for same image", func(t *testing.T) {
		storage := newMockStorage()
		server := &mockServer{}
		cache, err := NewCachingServer(server, storage, 10)
		if err != nil {
			t.Fatalf("Failed to create caching server: %v", err)
		}

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
		require.Equal(t, 1, server.callCount, "Expected 1 parent server call (from concurrent requests)")
	})
}
