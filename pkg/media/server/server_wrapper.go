package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"

	"golang.org/x/sync/semaphore"
)

// requestKey uniquely identifies a request by filename and class
type requestKey struct {
	filename string
	class    string
}

// inFlightRequest represents a request that is currently being processed
type inFlightRequest struct {
	done   chan struct{}
	buffer *bytes.Buffer
	mime   string
	err    error
}

// ServerWrapper wraps the media Server to provide request deduplication and concurrency limiting
type ServerWrapper struct {
	server        MediaServer
	inFlight      map[requestKey]*inFlightRequest
	mu            sync.Mutex
	sem           *semaphore.Weighted
	maxConcurrent int64
}

// NewWrapper creates a new ServerWrapper with the given concurrency limit
func NewWrapper(server MediaServer, maxConcurrent int64) *ServerWrapper {
	return &ServerWrapper{
		server:        server,
		inFlight:      make(map[requestKey]*inFlightRequest),
		sem:           semaphore.NewWeighted(maxConcurrent),
		maxConcurrent: maxConcurrent,
	}
}

// GetImage implements the same interface as Server.GetImage but with deduplication and concurrency limiting
func (w *ServerWrapper) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	key := requestKey{filename: fname, class: class}

	// Check if there's an in-flight request under mutex lock
	w.mu.Lock()
	if req, exists := w.inFlight[key]; exists {
		w.mu.Unlock()
		// Wait for the in-flight request to complete
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-req.done:
			return bytes.NewReader(req.buffer.Bytes()), req.mime, req.err
		}
	}

	// Create a new in-flight request
	req := &inFlightRequest{
		done:   make(chan struct{}),
		buffer: &bytes.Buffer{},
	}
	w.inFlight[key] = req
	w.mu.Unlock()

	// Clean up when we're done
	defer func() {
		w.mu.Lock()
		delete(w.inFlight, key)
		w.mu.Unlock()
		close(req.done)
	}()

	// Now acquire the semaphore since we'll actually process the request
	if err := w.sem.Acquire(ctx, 1); err != nil {
		return nil, "", err
	}
	defer w.sem.Release(1)

	// Process the request
	reader, mime, err := w.server.GetImage(ctx, fname, class)
	if err != nil {
		req.err = err
		return nil, "", err
	}

	// Buffer the response so it can be reused
	if _, err := io.Copy(req.buffer, reader); err != nil {
		req.err = err
		return nil, "", err
	}

	req.mime = mime
	return bytes.NewReader(req.buffer.Bytes()), mime, nil
}

// ServeImage delegates to the underlying server's ServeImage implementation
func (w *ServerWrapper) ServeImage(ctx context.Context, req *http.Request, resp http.ResponseWriter, fname string) error {
	return w.server.ServeImage(ctx, req, resp, fname)
}
