package server

import (
	"context"
	"io"
	"net/http"
)

type MediaStorage interface {
	UploadFile(ctx context.Context, fname string, b []byte, contentType string) error
	DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error)
}

type MediaServer interface {
	ServeImage(ctx context.Context, w http.ResponseWriter, fname string)
}
