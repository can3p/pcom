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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMediaServer implements MediaServer interface for testing
type mockMediaServer struct {
	mu            sync.Mutex
	callCount     int
	responseDelay time.Duration
	shouldError   bool
	readErr       error
}

// errorReader is an io.Reader that always fails, used to force io.Copy to
// return an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
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

	if m.readErr != nil {
		return &errorReader{err: m.readErr}, "image/jpeg", nil
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

func TestServerWrapper_GetImage_Errors(t *testing.T) {
	readErr := errors.New("read failed")

	cases := []struct {
		name string
		mock *mockMediaServer
		ctx  func() (context.Context, context.CancelFunc)
		want func(t *testing.T, err error)
	}{
		{
			name: "handles context cancellation",
			mock: &mockMediaServer{responseDelay: 100 * time.Millisecond},
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
			want: func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		{
			name: "handles server errors",
			mock: &mockMediaServer{shouldError: true},
			ctx:  func() (context.Context, context.CancelFunc) { return context.Background(), func() {} },
			want: func(t *testing.T, err error) { require.Error(t, err) },
		},
		{
			// The underlying reader fails mid-copy; GetImage must propagate
			// that error rather than returning a successful, truncated response.
			name: "propagates an io.Copy read error",
			mock: &mockMediaServer{readErr: readErr},
			ctx:  func() (context.Context, context.CancelFunc) { return context.Background(), func() {} },
			want: func(t *testing.T, err error) { require.ErrorIs(t, err, readErr) },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wrapper := NewWrapper(c.mock, 1)
			ctx, cancel := c.ctx()
			defer cancel()

			_, _, err := wrapper.GetImage(ctx, "test.jpg", "thumbnail")
			c.want(t, err)
		})
	}
}

func TestServerWrapper_GetImage(t *testing.T) {
	t.Run("deduplicates concurrent requests", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 100 * time.Millisecond}
		wrapper := NewWrapper(mock, 10)

		var wg sync.WaitGroup
		for range 5 {
			wg.Go(func() {
				// assert, not require: FailNow must not run off the test goroutine.
				reader, mime, err := wrapper.GetImage(context.Background(), "test.jpg", "thumbnail")
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, "image/jpeg", mime)
				data, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, "mock image data", string(data))
			})
		}
		wg.Wait()

		require.Equal(t, 1, mock.getCallCount(), "expected 1 call to underlying server")
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 100 * time.Millisecond}
		wrapper := NewWrapper(mock, 2)

		start := time.Now()
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Go(func() {
				_, _, err := wrapper.GetImage(context.Background(), "test.jpg", "class"+string(rune(i)))
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		// With 6 different requests and a concurrency limit of 2, it should
		// take at least 300ms (3 batches * 100ms).
		require.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond)
	})

	t.Run("cleans up the in-flight map after each request", func(t *testing.T) {
		mock := &mockMediaServer{}
		wrapper := NewWrapper(mock, 1)

		for range 10 {
			_, _, err := wrapper.GetImage(context.Background(), "test.jpg", "thumbnail")
			require.NoError(t, err)

			wrapper.mu.Lock()
			inFlight := len(wrapper.inFlight)
			wrapper.mu.Unlock()
			require.Empty(t, inFlight, "in-flight map not cleaned up")
		}
	})

	t.Run("different files don't share requests", func(t *testing.T) {
		mock := &mockMediaServer{responseDelay: 50 * time.Millisecond}
		wrapper := NewWrapper(mock, 10)

		var wg sync.WaitGroup
		for _, fname := range []string{"file1.jpg", "file2.jpg"} {
			wg.Go(func() {
				_, _, err := wrapper.GetImage(context.Background(), fname, "thumb")
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		require.Equal(t, 2, mock.getCallCount(), "expected 2 calls to underlying server")
	})
}
