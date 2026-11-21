package app

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// setupNotesApi mounts the notes API, the one part of the API a notes key
// opens: r admits notes keys as well as full ones.
func setupNotesApi(r *gin.RouterGroup, d *Deps) {
	svc := d.Services.Notes
	site := siteOf(d)

	r.POST("/notes", func(c *gin.Context) {
		web.ApiNotesRespond(c, web.ApiCreateNote(c, svc, auth.GetAPIUserData(c).DBUser, site))
	})

	r.GET("/notes", func(c *gin.Context) {
		web.ApiNotesRespond(c, web.ApiGetNotes(c, svc, auth.GetAPIUserData(c).DBUser, site))
	})
}
