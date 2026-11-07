package app

import (
	"net/http"
	"time"

	mediasvc "github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
)

// Defaults of the request body caps, used when Config leaves them zero.
const (
	// DefaultMaxBodySize is the most bytes of a request body other than an
	// upload: it fits the largest form, a post body of
	// posts.DefaultPostBodyMaxLength characters, percent-encoded.
	DefaultMaxBodySize = 1 << 20
	// DefaultMaxImportSize is the most bytes of a blog archive to import.
	DefaultMaxImportSize = 100 << 20
)

// Paths of the routes that take a file, which get a larger body cap than
// every other route.
const (
	uploadMediaPath = "/controls/action/upload_media"
	apiImagePath    = "/api/v1/image"
	importPath      = "/controls/action/settings/import"
	// The notes captures: a recording, photos and text in one request.
	noteAudioPath   = "/controls/action/note_audio"
	noteCapturePath = "/controls/action/note_capture"
	noteSharePath   = "/controls/notes/share"
	apiNotesPath    = "/api/v1/notes"
)

// bodyLimits caps every request body before anything reads it: login,
// signup and the other forms open to anyone included. The upload routes take
// a file of up to maxUpload (maxImport for an import, maxCapture for a notes
// capture with all its files) bytes, plus maxBody for the rest of the form.
func bodyLimits(maxBody, maxUpload, maxImport, maxCapture int64) gin.HandlerFunc {
	if maxBody <= 0 {
		maxBody = DefaultMaxBodySize
	}

	if maxUpload <= 0 {
		maxUpload = mediasvc.DefaultMaxUploadSize
	}

	if maxImport <= 0 {
		maxImport = DefaultMaxImportSize
	}

	if maxCapture <= 0 {
		maxCapture = notes.DefaultCaptureMaxBytes
	}

	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}

		limit := maxBody

		switch c.FullPath() {
		case uploadMediaPath, apiImagePath:
			limit += maxUpload
		case importPath:
			limit += maxImport
		case noteAudioPath, noteCapturePath, noteSharePath, apiNotesPath:
			limit += maxCapture
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// Defaults of the server timeouts, used when ServerTimeouts leaves them zero.
const (
	DefaultReadHeaderTimeout = 10 * time.Second
	DefaultReadTimeout       = 60 * time.Second
	// DefaultWriteTimeout is above claude.DefaultTimeout: the proofread
	// actions wait that long on the Anthropic API.
	DefaultWriteTimeout = 120 * time.Second
	DefaultIdleTimeout  = 120 * time.Second
)

// ServerTimeouts are the http.Server timeouts of the app; a zero field keeps
// its default.
type ServerTimeouts struct {
	ReadHeader, Read, Write, Idle time.Duration
}

// NewServer is the app's HTTP server on addr. Unlike a zero http.Server it
// has timeouts, so a client sending a request or reading a response a byte
// at a time can't hold a connection forever.
func NewServer(addr string, handler http.Handler, t ServerTimeouts) *http.Server {
	orDefault := func(d, def time.Duration) time.Duration {
		if d <= 0 {
			return def
		}

		return d
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: orDefault(t.ReadHeader, DefaultReadHeaderTimeout),
		ReadTimeout:       orDefault(t.Read, DefaultReadTimeout),
		WriteTimeout:      orDefault(t.Write, DefaultWriteTimeout),
		IdleTimeout:       orDefault(t.Idle, DefaultIdleTimeout),
	}
}
