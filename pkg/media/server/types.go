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
	GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error)
	ServeImage(ctx context.Context, getter MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error
}

type MediaGetter interface {
	GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error)
}
