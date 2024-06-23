package main

import (
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/media"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func setupApi(r *gin.RouterGroup, db *sqlx.DB, mediaServer media.MediaServer) {
	r.GET("/me", func(c *gin.Context) {
		userData := auth.GetAPIUserData(c)

		c.JSON(http.StatusOK, gin.H{
			"handle": userData.DBUser.Username,
		})
	})
}
