package server

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/can3p/pcom/pkg/media/server/encoder"
	"github.com/disintegration/imaging"
)

type ClassParams struct {
	Width  int
	Height int
}

type Server struct {
	storage MediaStorage
	encoder *encoder.Encoder
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

func New(storage MediaStorage, o ...Option) *Server {
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
		encoder: &encoder.Encoder{},
		options: opts,
	}
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

	img, err := s.encoder.Decode(dl)

	if err != nil {
		return nil, "", err
	}

	if params, ok := s.options.classMap[class]; ok {
		img = imaging.Resize(img, params.Width, params.Height, imaging.Lanczos)
	}

	return s.encoder.Encode(img)
}

func (s Server) ServeImage(ctx context.Context, req *http.Request, w http.ResponseWriter, fname string) error {
	class := s.options.classResolver(ctx, req)

	// we control all the classes, never allow to
	// enumerate them to ddos pod
	if _, ok := s.options.classMap[class]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}

	out, ct, err := s.GetImage(ctx, fname, class)

	if err != nil {
		return err
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
