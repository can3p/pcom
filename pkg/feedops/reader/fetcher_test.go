package reader_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/feedops/reader"
	"github.com/stretchr/testify/require"
)

func TestNewFetcher(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{}
	fetcher := reader.NewFetcher(httpClient)
	require.NotNil(t, fetcher)
}

func TestFetch_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rssFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>A test feed</description>
    <item>
      <title>First Post</title>
      <description>First description</description>
      <link>https://example.com/post1</link>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Second Post</title>
      <description>Second description</description>
      <link>https://example.com/post2</link>
      <pubDate>Tue, 02 Jan 2024 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	feed, err := fetcher.Fetch(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, "Test Feed", feed.Title)
	require.Equal(t, "A test feed", feed.Description)
	require.Len(t, feed.Items, 2)
	require.Equal(t, "First Post", feed.Items[0].Title)
	require.Equal(t, "Second Post", feed.Items[1].Title)
}

func TestFetch_ContextCanceled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("<rss></rss>"))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetcher.Fetch(ctx, server.URL)
	require.Error(t, err)
}

func TestFetchMedia_Success(t *testing.T) {
	t.Parallel()

	// Create a valid PNG file (minimal valid PNG)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, // IHDR chunk length
		0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x00, 0x01, // width: 1
		0x00, 0x00, 0x00, 0x01, // height: 1
		0x08, 0x02, 0x00, 0x00, 0x00, // bit depth, color type, compression, filter, interlace
		0x90, 0x77, 0x53, 0xDE, // CRC
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngData)))
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	rc, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, pngData, data)
}

func TestFetchMedia_Timeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Simulate slow server
		_, _ = w.Write([]byte("test data"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Millisecond}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.Error(t, err)
	// Note: The timeout error handling in FetchMedia checks for context.DeadlineExceeded,
	// so we verify that an error occurred. The exact error might be wrapped.
}

func TestFetchMedia_SizeCapExceeded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Report a content-length larger than MaxMediaSize
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", reader.MaxMediaSize+1))
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.Error(t, err)
	require.ErrorIs(t, err, reader.ErrMediaTooLarge)
}

func TestFetchMedia_InvalidMIMEType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("This is not an image"))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.Error(t, err)
}

func TestFetchMedia_NonOKStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"404 Not Found", http.StatusNotFound},
		{"403 Forbidden", http.StatusForbidden},
		{"500 Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := &http.Client{}
			fetcher := reader.NewFetcher(client)

			_, err := fetcher.FetchMedia(context.Background(), server.URL)
			require.Error(t, err)
		})
	}
}

func TestFetchMedia_Redirects(t *testing.T) {
	t.Parallel()

	// Create a valid PNG for the final response
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00,
		0x90, 0x77, 0x53, 0xDE,
	}

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	rc, err := fetcher.FetchMedia(context.Background(), redirectServer.URL)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, pngData, data)
}

func TestFetchMedia_MultipleImageFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		data        []byte
	}{
		{
			"PNG",
			"image/png",
			[]byte{
				0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
				0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
			}, // PNG signature
		},
		{
			"JPEG",
			"image/jpeg",
			[]byte{
				0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
				0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
			}, // JPEG SOI + APP0
		},
		{
			"WEBP",
			"image/webp",
			[]byte{
				0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00,
				0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4C,
			}, // RIFF header + WEBP signature
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Pad with zeros to ensure we have enough data for peeking
			data := make([]byte, 512)
			copy(data, tt.data)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				_, _ = w.Write(data)
			}))
			defer server.Close()

			client := &http.Client{}
			fetcher := reader.NewFetcher(client)

			rc, err := fetcher.FetchMedia(context.Background(), server.URL)
			require.NoError(t, err)
			defer func() { _ = rc.Close() }()

			result, err := io.ReadAll(rc)
			require.NoError(t, err)
			require.Len(t, result, 512)
		})
	}
}

func TestFetchMedia_LimitedReader(t *testing.T) {
	t.Parallel()

	// Create data larger than MaxMediaSize with valid JPEG header
	// Don't report Content-Length to bypass early size check
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	extraData := bytes.Repeat([]byte{0xFF}, reader.MaxMediaSize+1024-len(jpegHeader))
	largeData := append(jpegHeader, extraData...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		// Don't set Content-Length to bypass early check
		_, _ = w.Write(largeData)
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	rc, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	// The limitedReader should cap at exactly MaxMediaSize
	require.Equal(t, reader.MaxMediaSize, len(data))
}

func TestFetchMedia_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate very slow response
		select {
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte("test"))
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := fetcher.FetchMedia(ctx, server.URL)
	require.Error(t, err)
	// Either timeout or the internal deadline exceeded from FetchMedia
}

func TestFetchMedia_SmallFile(t *testing.T) {
	t.Parallel()

	// Create minimal valid JPEG data
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	rc, err := fetcher.FetchMedia(context.Background(), server.URL)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, jpegData, data)
}

func TestFetchMedia_EmptyFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Return empty body - this should fail MIME validation
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), server.URL)
	// Empty file won't have valid image header
	require.Error(t, err)
}

func TestFetchMedia_PartialRead(t *testing.T) {
	t.Parallel()

	// Create minimal JPEG header
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "1000")
		// Write less data than Content-Length, but with valid JPEG header
		_, _ = w.Write(jpegHeader)
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), server.URL)
	// Should succeed because we only peeked at the header for MIME validation
	require.NoError(t, err)
}

func TestFetch_EmptyFeed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rssFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Feed</title>
    <description>No items here</description>
  </channel>
</rss>`
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	feed, err := fetcher.Fetch(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, "Empty Feed", feed.Title)
	require.Empty(t, feed.Items)
}

func TestFetch_InvalidXML(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte("not valid xml"))
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.Fetch(context.Background(), server.URL)
	require.Error(t, err)
}

func TestFetch_NetworkError(t *testing.T) {
	t.Parallel()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.Fetch(context.Background(), "http://invalid-url-that-does-not-exist-xyz.invalid/feed")
	require.Error(t, err)
}

func TestFetchMedia_ReadHeaderTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		// Flush headers immediately
		w.(http.Flusher).Flush()
		// Then pause, simulating slow body transmission
		select {
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte{0xFF, 0xD8})
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	client := &http.Client{}
	fetcher := reader.NewFetcher(client)

	// Use a very short timeout to force deadline exceeded during read
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := fetcher.FetchMedia(ctx, server.URL)
	require.Error(t, err)
	// The error should be either timeout or deadline exceeded
}

func TestFetchMedia_DoErrorNonTimeout(t *testing.T) {
	t.Parallel()

	// Create a mock HTTP client that returns an error
	mockTransport := &mockTransport{
		err: fmt.Errorf("connection refused"),
	}
	client := &http.Client{Transport: mockTransport}
	fetcher := reader.NewFetcher(client)

	_, err := fetcher.FetchMedia(context.Background(), "http://example.com/image.jpg")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch media")
}

type mockTransport struct {
	err error
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{}, nil
}
