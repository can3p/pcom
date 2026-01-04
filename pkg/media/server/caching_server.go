package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	defaultCacheSize = 1000
	cacheKeyFormat   = "%s/%s" // format: class/filename
)

type CachingServer struct {
	MediaServer
	storage    MediaStorage
	cache      *lru.Cache[string, bool]
	uploadLock sync.Map // fine-grained lock for concurrent uploads of the same file
}

func NewCachingServer(server MediaServer, storage MediaStorage, cacheSize int) (*CachingServer, error) {
	if cacheSize <= 0 {
		cacheSize = defaultCacheSize
	}

	cache, err := lru.New[string, bool](cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	return &CachingServer{
		MediaServer: server,
		storage:     storage,
		cache:       cache,
	}, nil
}

func (s *CachingServer) getCacheKey(fname, class string) string {
	return fmt.Sprintf(cacheKeyFormat, class, fname)
}

func (s *CachingServer) getUploadLock(key string) *sync.Mutex {
	// Get or create a new mutex for this key
	value, _ := s.uploadLock.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (s *CachingServer) removeUploadLock(key string) {
	s.uploadLock.Delete(key)
}

func (s *CachingServer) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	cacheKey := s.getCacheKey(fname, class)

	slog.Debug("GetImage called", "cacheKey", cacheKey)

	// Check in-memory cache first
	if _, ok := s.cache.Get(cacheKey); ok {
		slog.Debug("Cache hit", "cacheKey", cacheKey)
		// File is known to exist in storage
		reader, _, mime, err := s.storage.DownloadFile(ctx, cacheKey)
		if err != nil {
			return nil, "", err
		}
		return reader, mime, nil
	}

	// Check if file exists in storage
	exists, err := s.storage.ObjectExists(ctx, cacheKey)
	if err != nil {
		slog.Error("Failed to check object existence", "error", err, "key", cacheKey)
	} else if exists {
		slog.Debug("Storage hit", "cacheKey", cacheKey)
		s.cache.Add(cacheKey, true)
		reader, _, mime, err := s.storage.DownloadFile(ctx, cacheKey)
		if err != nil {
			return nil, "", err
		}
		return reader, mime, nil
	}

	// Get upload lock for this file
	lock := s.getUploadLock(cacheKey)
	lock.Lock()
	slog.Debug("Acquired lock", "cacheKey", cacheKey)
	releaseLock := func() {
		lock.Unlock()
		slog.Debug("Released lock", "cacheKey", cacheKey)
		s.removeUploadLock(cacheKey)
	}

	// Check in-memory cache once more.
	// It could happen that the file has been just uploaded by another goroutine
	// while we were waiting for the lock
	// XXX: cache is local to the process, we can still have duplicates
	// in case of multiple replicas.
	// This is fine, since pcom is meant for small deployments and prefers
	// simplicity over battling all corner cases.
	// If you ever need multi-node support, implement cache with redis backend instead of in mem store
	if _, ok := s.cache.Get(cacheKey); ok {
		slog.Debug("File found in cache after acquiring lock", "cacheKey", cacheKey)
		// File is known to exist in storage
		reader, _, mime, err := s.storage.DownloadFile(ctx, cacheKey)
		if err != nil {
			return nil, "", err
		}
		releaseLock()
		return reader, mime, nil
	}

	// Double-check if file was uploaded while we were waiting
	exists, err = s.storage.ObjectExists(ctx, cacheKey)
	if err == nil && exists {
		slog.Debug("File found in storage after lock wait", "cacheKey", cacheKey)
		s.cache.Add(cacheKey, true)
		reader, _, mime, err := s.storage.DownloadFile(ctx, cacheKey)
		if err != nil {
			return nil, "", err
		}
		releaseLock()
		return reader, mime, nil
	}

	// Get the image from parent server
	reader, mime, err := s.MediaServer.GetImage(ctx, fname, class)
	if err != nil {
		releaseLock()
		return nil, "", err
	}

	// Read the entire image into memory
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		releaseLock()
		return nil, "", fmt.Errorf("failed to read image: %w", err)
	}

	// Upload to storage asynchronously
	bufCopy := buf.Bytes() // Make a copy for async upload
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		defer releaseLock()

		if err := s.storage.UploadFile(ctx, cacheKey, bufCopy, mime); err != nil {
			slog.Error("Failed to cache image", "error", err, "key", cacheKey)
			return
		}

		s.cache.Add(cacheKey, true)
		slog.Info("Image cached successfully", "key", cacheKey)
	}()
	// Return the image to the client immediately
	return bytes.NewReader(buf.Bytes()), mime, nil
}
