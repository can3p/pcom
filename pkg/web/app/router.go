package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/gogo/util/ginhelpers/csrf"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// New builds the gin engine: middleware, templates and every route group.
func New(d *Deps) *gin.Engine {
	db := d.DB

	if d.Services == nil {
		d.Services = registry.New(db, registry.Deps{Sender: d.Sender, MediaStorage: d.MediaStorage, Site: siteOf(d), SenderAddress: d.Config.SenderAddress, AdminAddress: d.Config.AdminAddress, ProfileAboutMaxLength: d.Config.ProfileAboutMaxLength, CommentMaxLength: d.Config.CommentMaxLength, PostBodyMaxLength: d.Config.PostBodyMaxLength, PostSubjectMaxLength: d.Config.PostSubjectMaxLength, PromptMaxLength: d.Config.PromptMaxLength, UserStylesMaxLength: d.Config.UserStylesMaxLength, PageSize: d.Config.PageSize, RSSLimit: d.Config.RSSLimit, CodeKey: d.Config.SessionSalt, Login: d.Config.Login})
	}

	store := pgsession.NewStore(db, []byte(d.Config.SessionSalt))
	store.Options(sessions.Options{
		Path: "/",
		// safari wouldn't allow to save secure cookie
		// if server works on localhost
		Secure:   d.Config.SecureCookies,
		HttpOnly: true,
		MaxAge:   24 * 3600 * 30, // make every session one month long
		SameSite: http.SameSiteLaxMode,
	})

	router := gin.Default()

	router.Use(ginhelpers.Configure(ginhelpers.Options{
		RedirectToLogin: auth.RedirectToLogin(d.Config.SessionSalt),
		ShowErrors:      d.Config.ShowErrors,
	}))

	router.MaxMultipartMemory = 8 << 20 // 8 MiB

	if d.Config.ReportPanics {
		router.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
			// not auth.GetUserData: routes outside the auth group have no
			// session, and its MustGet would panic inside the recovery
			user := auth.GetAPIUserData(c).DBUser

			if nerr := d.Services.Accounts.SendAdminMail(c, admin.PageFailure(d.Config.SenderAddress, d.Config.AdminAddress, c, err, user)); nerr != nil {
				log.Printf("failed to queue the page failure notification: %v", nerr)
			}
		}))
	} else {
		log.Println("Custom error reporter skipped")
	}

	router.SetFuncMap(funcmap(d.Config.StaticAsset, siteOf(d)))
	router.LoadHTMLGlob(fmt.Sprintf("%s/*.html", d.Config.HTMLDir))

	csrfCheck := csrf.CSRFMiddleware(func(c *gin.Context) string { return auth.GetUserData(c).CSRFToken })
	apiGroup := router.Group("/api/v1", func(c *gin.Context) { auth.AuthAPI(c, d.Services.Accounts) })
	r := router.Group("/", csp.New(csp.Options{HSTS: d.Config.HSTS, StaticCDN: d.Config.StaticCDN, MediaCDN: d.Config.MediaCDN}), sessions.Sessions("sess", store), func(c *gin.Context) { auth.Auth(c, d.Services.Accounts) })
	controls := r.Group("/controls", auth.EnforceAuth)
	actions := controls.Group("/action", csrfCheck)
	nonControlsForms := r.Group("/form", csrfCheck)
	controlsForms := controls.Group("/form", csrfCheck)

	mountRoutes(d, router, apiGroup, r, controls, actions, nonControlsForms, controlsForms)

	return router
}
