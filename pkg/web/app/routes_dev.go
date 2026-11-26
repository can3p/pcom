package app

import (
	"net/http"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/gin-gonic/gin"
)

// DevInfo is what the /dev page shows a developer: where the local services
// answer and how to log in. It is plain data from the composition root, so
// the app knows nothing of the seed data or the compose file. An empty
// address leaves its row out.
type DevInfo struct {
	// TommyURL is the mail sink's UI, where login codes land.
	TommyURL string
	// S3URL is the local bucket for user media.
	S3URL string
	// PostgresURL is the database as psql on the host reaches it.
	PostgresURL string
	// Accounts are the seeded users; each logs in with a code mailed to its
	// Email.
	Accounts []DevAccount
	// APIKey is the seeded API key.
	APIKey string
}

// DevAccount is one seeded user on the /dev page.
type DevAccount struct {
	Name, Email, Role string
}

// mountDevRoutes registers the developer pages under /dev when
// Config.DevRoutes is on; with it off nothing under /dev exists. The pages
// show configuration and the mails' samples only: no database and no session.
func mountDevRoutes(d *Deps, router *gin.Engine) {
	if !d.Config.DevRoutes {
		return
	}

	dev := router.Group("/dev")

	dev.GET("", func(c *gin.Context) {
		c.HTML(http.StatusOK, "dev--index.html", map[string]any{"SiteRoot": d.Config.SiteRoot, "Dev": d.Config.Dev})
	})

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
