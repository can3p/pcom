package app

import (
	"net/http"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/gin-gonic/gin"
)

// mountDevRoutes registers the developer pages under /dev when
// Config.DevRoutes is on; with it off nothing under /dev exists. The pages
// render the mails' samples only: no database and no session.
func mountDevRoutes(d *Deps, router *gin.Engine) {
	if !d.Config.DevRoutes {
		return
	}

	dev := router.Group("/dev")

	dev.GET("/mail", func(c *gin.Context) {
		c.HTML(http.StatusOK, "dev--mail-index.html", map[string]any{"Mails": mail.All()})
	})

	dev.GET("/mail/:sample", func(c *gin.Context) {
		name := c.Param("sample")

		for _, m := range mail.All() {
			for _, s := range m.Samples {
				if s.Name != name {
					continue
				}

				page := map[string]any{"Sample": s.Name, "Mail": m.Name, "Err": s.Err}
				if s.Envelope != nil && s.Envelope.Mail != nil {
					page["Subject"] = s.Envelope.Mail.Subject
					page["Text"] = s.Envelope.Mail.Text
					page["HTML"] = s.Envelope.Mail.Html
				}

				c.HTML(http.StatusOK, "dev--mail-sample.html", page)

				return
			}
		}

		c.AbortWithStatus(http.StatusNotFound)
	})
}
