// Package fakestorage is an in-memory implementation of
// github.com/can3p/pcom/pkg/media/server.MediaStorage, for tests that
// exercise upload/download code without a real object store.
package fakestorage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
)

// compile-time check that Storage implements server.MediaStorage.
var _ server.MediaStorage = (*Storage)(nil)

type object struct {
	data        []byte
	contentType string
}

// Storage is an in-memory server.MediaStorage. Use New to construct one; it
// is safe for concurrent use.
type Storage struct {
	mu      sync.Mutex
	objects map[string]object

	uploadErr   error
	downloadErr error
	existsErr   error
}

// New returns an empty Storage.
func New() *Storage {
	return &Storage{objects: map[string]object{}}
}

// FailUploadWith makes UploadFile return err. Pass nil to clear it.
func (s *Storage) FailUploadWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.uploadErr = err
}

// FailDownloadWith makes DownloadFile return err. Pass nil to clear it.
func (s *Storage) FailDownloadWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.downloadErr = err
}

// FailExistsWith makes ObjectExists return err. Pass nil to clear it.
func (s *Storage) FailExistsWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.existsErr = err
}

// UploadFile implements server.MediaStorage.
func (s *Storage) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.uploadErr != nil {
		return s.uploadErr
	}

	cp := make([]byte, len(b))
	copy(cp, b)

	if contentType == "" {
		contentType = http.DetectContentType(cp)
	}

	s.objects[fname] = object{data: cp, contentType: contentType}

	return nil
}

// DownloadFile implements server.MediaStorage. A missing file returns
// media.ErrNotFound, the same sentinel the real storage backends use.
func (s *Storage) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.downloadErr != nil {
		return nil, 0, "", s.downloadErr
	}

	obj, ok := s.objects[fname]
	if !ok {
		return nil, 0, "", media.ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(obj.data)), int64(len(obj.data)), obj.contentType, nil
}

// ObjectExists implements server.MediaStorage.
func (s *Storage) ObjectExists(ctx context.Context, fname string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.existsErr != nil {
		return false, s.existsErr
	}

	_, ok := s.objects[fname]

	return ok, nil
}
