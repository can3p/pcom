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

	// Check in-memory cache first
	if _, ok := s.cache.Get(cacheKey); ok {
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
	defer func() {
		lock.Unlock()
		s.removeUploadLock(cacheKey)
	}()

	// Double-check if file was uploaded while we were waiting
	exists, err = s.storage.ObjectExists(ctx, cacheKey)
	if err == nil && exists {
		s.cache.Add(cacheKey, true)
		reader, _, mime, err := s.storage.DownloadFile(ctx, cacheKey)
		if err != nil {
			return nil, "", err
		}
		return reader, mime, nil
	}

	// Get the image from parent server
	reader, mime, err := s.MediaServer.GetImage(ctx, fname, class)
	if err != nil {
		return nil, "", err
	}

	// Read the entire image into memory
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, "", fmt.Errorf("failed to read image: %w", err)
	}

	// Upload to storage asynchronously
	bufCopy := buf.Bytes() // Make a copy for async upload
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

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
