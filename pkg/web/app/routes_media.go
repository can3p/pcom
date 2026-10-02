package app

import (
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/media"
	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
	"github.com/gin-gonic/gin"
)

// staticCacheControl is sent with every /static file that is served.
const staticCacheControl = "public, max-age=604800, immutable, stale-while-revalidate=86400"

// successCacheWriter sets Cache-Control only on a response that is not an
// error, so a missing asset's 404 is not cached as immutable (#183).
type successCacheWriter struct {
	gin.ResponseWriter
	value string
}

func (w *successCacheWriter) apply(code int) {
	if code < http.StatusBadRequest {
		w.Header().Set("Cache-Control", w.value)
	} else {
		w.Header().Del("Cache-Control")
	}
}

func (w *successCacheWriter) WriteHeader(code int) {
	if !w.Written() {
		w.apply(code)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *successCacheWriter) WriteHeaderNow() {
	if !w.Written() {
		w.apply(w.Status())
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *successCacheWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	return w.ResponseWriter.Write(data)
}

func (w *successCacheWriter) WriteString(s string) (int, error) {
	w.WriteHeaderNow()
	return w.ResponseWriter.WriteString(s)
}

// mountMediaRoutes serves the frontend build and user media.
func mountMediaRoutes(d *Deps, router *gin.Engine) {
	mediaServer := d.MediaServer
	staticAsset := d.Config.StaticAsset

	//cache static forever
	if d.Config.StaticCache {
		router.Group("/static", func(c *gin.Context) {
			c.Writer = &successCacheWriter{ResponseWriter: c.Writer, value: staticCacheControl}
			c.Next()
		}).Static("/", "dist")
	} else {
		router.Group("/static").Static("/", "dist")
	}

	router.GET("user-media/:fname", func(c *gin.Context) {
		switch c.Param("fname") {
		case "robots.txt":
			c.String(http.StatusOK, "OK")
		case "favicon.ico":
			c.Redirect(http.StatusMovedPermanently, staticAsset("static/favicon.ico"))
		default:
			c.Status(http.StatusNotFound)
		}
	})

	router.GET("user-media/:fname/:class", func(c *gin.Context) {
		fname := c.Param("fname")

		if fname == "" {
			c.Status(http.StatusNotFound)
			return
		}

		if fname == "robots.txt" {
			c.String(http.StatusOK, "OK")
			return
		}

		if fname == "favicon.ico" {
			c.Redirect(http.StatusMovedPermanently, staticAsset("static/favicon.ico"))
			return
		}

		err := mediaServer.ServeImage(c, mediaServer, c.Request, c.Writer, fname)

		if err != nil {
			if errors.Is(err, mediaerrors.ErrNotFound) || errors.Is(err, media.ErrNotFound) {
				c.Status(http.StatusNotFound)
				return
			}

			panic(err)
		}
	})
}
