package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// mockMediaServer implements MediaServer interface for testing
type mockMediaServer struct {
	mu            sync.Mutex
	callCount     int
	responseDelay time.Duration
	shouldError   bool
}

func (m *mockMediaServer) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.responseDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(m.responseDelay):
		}
	}

	if m.shouldError {
		return nil, "", errors.New("mock error")
	}

	return bytes.NewReader([]byte("mock image data")), "image/jpeg", nil
}

func (m *mockMediaServer) ServeImage(ctx context.Context, getter MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error {
	return nil
}

func (m *mockMediaServer) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestServerWrapper_GetImage(t *testing.T) {
	t.Run("deduplicates concurrent requests", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 100 * time.Millisecond}
		wrapper := NewWrapper(mock, 10)

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reader, mime, err := wrapper.GetImage(context.Background(), "test.jpg", "thumbnail")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mime != "image/jpeg" {
					t.Errorf("unexpected mime type: %s", mime)
				}
				data, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("failed to read response: %v", err)
					return
				}
				if string(data) != "mock image data" {
					t.Errorf("unexpected response data: %s", string(data))
				}
			}()
		}
		wg.Wait()

		if count := mock.getCallCount(); count != 1 {
			t.Errorf("expected 1 call to underlying server, got %d", count)
		}
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 100 * time.Millisecond}
		wrapper := NewWrapper(mock, 2)

		start := time.Now()
		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, err := wrapper.GetImage(context.Background(), "test.jpg", "class"+string(rune(i)))
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}(i)
		}
		wg.Wait()

		// With 6 different requests and concurrency limit of 2,
		// it should take at least 300ms (3 batches * 100ms)
		duration := time.Since(start)
		if duration < 300*time.Millisecond {
			t.Errorf("requests completed too quickly, expected at least 300ms, got %v", duration)
		}
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 100 * time.Millisecond}
		wrapper := NewWrapper(mock, 1)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, _, err := wrapper.GetImage(ctx, "test.jpg", "thumbnail")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected deadline exceeded error, got %v", err)
		}
	})

	t.Run("handles server errors", func(t *testing.T) {
		mock := &mockMediaServer{shouldError: true}
		wrapper := NewWrapper(mock, 1)

		_, _, err := wrapper.GetImage(context.Background(), "test.jpg", "thumbnail")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("memory cleanup", func(t *testing.T) {
		mock := &mockMediaServer{}
		wrapper := NewWrapper(mock, 1)

		// Make several requests and verify that in-flight map is cleaned up
		for i := 0; i < 10; i++ {
			_, _, err := wrapper.GetImage(context.Background(), "test.jpg", "thumbnail")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			wrapper.mu.Lock()
			if len(wrapper.inFlight) != 0 {
				t.Errorf("in-flight map not cleaned up, contains %d entries", len(wrapper.inFlight))
			}
			wrapper.mu.Unlock()
		}
	})
}
