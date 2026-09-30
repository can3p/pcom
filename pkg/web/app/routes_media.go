package app

import (
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/media"
	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
	"github.com/gin-gonic/gin"
)

// mountMediaRoutes serves the frontend build and user media.
func mountMediaRoutes(d *Deps, router *gin.Engine) {
	mediaServer := d.MediaServer
	staticAsset := d.Config.StaticAsset

	//cache static forever
	if d.Config.StaticCache {
		router.Group("/static", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=604800, immutable, stale-while-revalidate=86400")
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
