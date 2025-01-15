package server

import (
	"bytes"
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"

	"github.com/davidbyttow/govips/v2/vips"
)

type ClassParams struct {
	Width  int
	Height int
}

type Server struct {
	storage MediaStorage
	options options
}

type ClassResolver func(context.Context, *http.Request) string

type options struct {
	classResolver ClassResolver
	classMap      map[string]ClassParams
	addHeaders    http.Header
}

func defaultClassResolver(c context.Context, req *http.Request) string {
	return req.URL.Query().Get("class")
}

type Option func(s *options)

func WithClassResolver(r ClassResolver) Option {
	return func(o *options) {
		o.classResolver = r
	}
}

func WithClass(name string, params ClassParams) Option {
	return func(o *options) {
		o.classMap[name] = params
	}
}

func WithPermaCache(enabled bool) Option {
	return func(o *options) {
		if enabled {
			o.addHeaders.Add("Cache-Control", "public, max-age=604800, immutable, stale-while-revalidate=86400")
		}
	}
}

func New(storage MediaStorage, o ...Option) (*Server, func()) {
	vips.Startup(nil)

	opts := options{
		classResolver: defaultClassResolver,
		classMap:      map[string]ClassParams{},
		addHeaders:    http.Header{},
	}

	for _, o := range o {
		o(&opts)
	}

	return &Server{
		storage: storage,
		options: opts,
	}, vips.Shutdown
}

// get the reader with downloaded and transformed image after all transformations
func (s Server) GetImage(ctx context.Context, fname string, class string) (io.Reader, string, error) {
	dl, _, _, err := s.storage.DownloadFile(ctx, fname)

	if err != nil {
		return nil, "", err
	}

	defer func() {
		if err := dl.Close(); err != nil {
			log.Printf("Error closing download: %v", err)
		}
	}()

	img, err := vips.NewImageFromReader(dl)

	if err != nil {
		return nil, "", err
	}

	if params, ok := s.options.classMap[class]; ok {
		// Resize to fit within the bounding box while maintaining aspect ratio
		var scale float64

		imgWidth := img.Width()
		imgHeight := img.Height()

		// Calculate the scale factor to fit within the bounding box
		widthScale := float64(params.Width) / float64(imgWidth)
		heightScale := float64(params.Height) / float64(imgHeight)

		// Use the smaller scale to ensure image fits within bounds
		if widthScale < heightScale {
			scale = widthScale
		} else {
			scale = heightScale
		}

		if scale < 1.0 {
			err = img.Resize(scale, vips.KernelLanczos3)
			if err != nil {
				return nil, "", err
			}
		}
	}

	ep := vips.NewDefaultWEBPExportParams()

	b, _, err := img.Export(ep)
	if err != nil {
		return nil, "", err
	}

	return bytes.NewReader(b), "image/webp", nil
}

func (s Server) ServeImage(ctx context.Context, getter MediaGetter, req *http.Request, w http.ResponseWriter, fname string) error {
	class := s.options.classResolver(ctx, req)

	// we control all the classes, never allow to
	// enumerate them to ddos pod
	if _, ok := s.options.classMap[class]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}

	out, ct, err := getter.GetImage(ctx, fname, class)

	if err != nil {
		return err
	}

	// Close the reader if it implements io.Closer
	if closer, ok := out.(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				slog.Warn("Failed to close the reader", "err", err)
			}
		}()
	}

	w.Header().Set("Content-Type", ct)

	for name, headers := range s.options.addHeaders {
		for _, h := range headers {
			w.Header().Set(name, h)
		}
	}

	_, err = io.Copy(w, out)

	return err

}
