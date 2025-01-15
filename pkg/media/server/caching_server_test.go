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
	storage := newMockStorage()
	server := &mockServer{}
	cache, err := NewCachingServer(server, storage, 10)
	if err != nil {
		t.Fatalf("Failed to create caching server: %v", err)
	}

	t.Run("first request fetches from parent and caches", func(t *testing.T) {
		reader, mime, err := cache.GetImage(ctx, "test.jpg", "thumb")
		if err != nil {
			t.Fatalf("Failed to get image: %v", err)
		}
		if mime != "image/webp" {
			t.Errorf("Expected mime type image/webp, got %s", mime)
		}

		// Read the image
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read image: %v", err)
		}
		if string(data) != "test image" {
			t.Errorf("Expected 'test image', got %s", string(data))
		}

		// Wait for async cache
		time.Sleep(100 * time.Millisecond)

		if storage.callCount.upload != 1 {
			t.Errorf("Expected 1 upload, got %d", storage.callCount.upload)
		}
		if server.callCount != 1 {
			t.Errorf("Expected 1 parent server call, got %d", server.callCount)
		}
	})

	t.Run("subsequent requests use cache", func(t *testing.T) {
		reader, _, err := cache.GetImage(ctx, "test.jpg", "thumb")
		if err != nil {
			t.Fatalf("Failed to get image: %v", err)
		}
		_, _ = io.ReadAll(reader) // Read to close

		if server.callCount != 1 {
			t.Errorf("Expected parent server call count to remain 1, got %d", server.callCount)
		}
		if storage.callCount.download != 1 {
			t.Errorf("Expected 1 download from storage, got %d", storage.callCount.download)
		}
	})

	t.Run("concurrent requests for same image", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reader, _, err := cache.GetImage(ctx, "new.jpg", "thumb")
				if err != nil {
					t.Errorf("Failed to get image: %v", err)
					return
				}
				_, _ = io.ReadAll(reader) // Read to close
			}()
		}
		wg.Wait()

		// Should only call parent server once
		if server.callCount != 2 { // 1 from previous tests + 1 from concurrent requests
			t.Errorf("Expected 2 parent server calls, got %d", server.callCount)
		}
	})

	t.Run("memory cache hit", func(t *testing.T) {
		existsCount := storage.callCount.exists
		reader, _, err := cache.GetImage(ctx, "test.jpg", "thumb")
		if err != nil {
			t.Fatalf("Failed to get image: %v", err)
		}
		_, _ = io.ReadAll(reader) // Read to close

		if storage.callCount.exists != existsCount {
			t.Errorf("Expected no additional exists checks, got %d new checks", storage.callCount.exists-existsCount)
		}
	})
}
