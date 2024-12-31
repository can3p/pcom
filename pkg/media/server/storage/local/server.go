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

	"github.com/can3p/pcom/pkg/media"
)

type localServer struct {
	path string
}

func NewLocalServer(path string) (*localServer, error) {
	stat, err := os.Stat(path)

	fileMissing := errors.Is(err, os.ErrNotExist)

	if err != nil && !fileMissing {
		return nil, err
	}

	if !fileMissing && !stat.IsDir() {
		return nil, fmt.Errorf("Path [%s] exists and is not a folder", path)
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
	filePath := path.Join(ls.path, fname)

	_, err := os.Stat(filePath)

	fileMissing := errors.Is(err, os.ErrNotExist)

	if err != nil && !fileMissing {
		return err
	}

	if !fileMissing {
		return fmt.Errorf("File [%s] already exits, will not overwrite", fname)
	}

	return os.WriteFile(filePath, b, 0644)
}

func (ls *localServer) DownloadFile(ctx context.Context, fname string) (io.Reader, int64, string, error) {
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

	return reader, int64(len(b)), ftype, nil
}
