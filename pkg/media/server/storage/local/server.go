package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/can3p/pcom/pkg/media"
)

type localServer struct {
	path string
	mu   sync.Mutex // protect concurrent writes to the same file
}

func NewLocalServer(path string) (*localServer, error) {
	stat, err := os.Stat(path)

	fileMissing := errors.Is(err, os.ErrNotExist)

	if err != nil && !fileMissing {
		return nil, err
	}

	if !fileMissing && !stat.IsDir() {
		return nil, fmt.Errorf("path [%s] exists and is not a folder", path)
	}

	if fileMissing {
		err := os.MkdirAll(path, 0755)

		if err != nil {
			return nil, err
		}
	}

	return &localServer{
		path: path,
	}, nil
}

func (ls *localServer) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	filePath := path.Join(ls.path, fname)
	dirPath := filepath.Dir(filePath)

	// Create directory structure if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	_, err := os.Stat(filePath)
	fileMissing := errors.Is(err, os.ErrNotExist)

	if err != nil && !fileMissing {
		return err
	}

	if !fileMissing {
		return fmt.Errorf("file [%s] already exits, will not overwrite", fname)
	}

	// Create a temporary file first
	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, b, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Rename is atomic on POSIX systems
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile) // Clean up temp file if rename fails
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func (ls *localServer) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	filePath := path.Join(ls.path, fname)

	b, err := os.ReadFile(filePath)

	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, "", media.ErrNotFound
		}

		return nil, 0, "", err
	}

	ftype := http.DetectContentType(b)
	reader := bytes.NewReader(b)

	return io.NopCloser(reader), int64(len(b)), ftype, nil
}

func (ls *localServer) ObjectExists(ctx context.Context, fname string) (bool, error) {
	filePath := path.Join(ls.path, fname)
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
