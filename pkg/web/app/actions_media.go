package app

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountMediaActions registers the media upload action.
func mountMediaActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB
	mediaStorage := d.MediaStorage

	r.POST("/upload_media", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		res := web.ApiUploadImage(c, db, userData.DBUser, mediaStorage)

		if res.IsError() {
			reportError(c, fmt.Sprintf("Operation Failed: %s", res.Error()))
			return
		}

		resp := res.MustGet()

		c.JSON(http.StatusOK, gin.H{
			"uploaded_url": resp.ImageID,
		})
	})
}
