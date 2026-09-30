package app

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/gin-gonic/gin"
)

// mountMediaActions registers the media upload action.
func mountMediaActions(d *Deps, r *gin.RouterGroup) {
	r.POST("/upload_media", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		file, err := c.FormFile("file")
		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		f, err := file.Open()
		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		fname, err := d.Services.Media.Upload(c, userData.DBUser, f)
		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"uploaded_url": fname,
		})
	})
}
