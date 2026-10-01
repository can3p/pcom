package app

import (
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// mountPostRoutes registers the pages and forms that write posts, comments and prompts, and the post export.
func mountPostRoutes(d *Deps, r, controlsForms *gin.RouterGroup) {
	posts := d.Services.Posts

	r.GET("/posts/:id/zip", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)
		postID := c.Param("id")

		// anyone who may see the post may export it, so the visibility
		// check is the one /posts/:id uses. The viewer may be anonymous.
		post, err := d.Services.Reading.Post(c, userData.DBUser, postID, false)
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		author := post.Post.Author

		b, err := posts.ExportPost(c, author.ID, postID)

		if err != nil {
			panic(err)
		}

		sendZip(c, author.Username, b)
	})

	mountTranslateRoutes(d, r)

	r.GET("/posts/:id/edit", auth.EnforceAuth, requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "edit_post.html", web.EditPost(c, posts, &userData, c.Param("id")))
	})

	r.GET("/write", auth.EnforceAuth, func(c *gin.Context) {
		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "write.html", web.Write(c, posts, &userData))
	})

	controlsForms.POST("/edit_post", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		postID := c.PostForm("post_id")

		var form *forms.PostForm
		var err error

		if postID == "" {
			form, err = forms.NewPostFormNew(c, posts, dbUser, c.PostForm("prompt_id"))

			if err != nil {
				panic(err)
			}
		} else {
			form, err = forms.EditPostFormNew(c, posts, dbUser, postID)

			if err != nil {
				if err == service.ErrNotFound {
					c.Status(http.StatusNotFound)
					return
				}

				panic(err)
			}
		}

		gogoForms.DefaultHandler(c, form)
	})

	controlsForms.POST("/new_comment", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		form := forms.NewCommentFormNew(posts, dbUser, c.PostForm("post_id"))

		gogoForms.DefaultHandler(c, form)
	})

	controlsForms.POST("/edit_comment/:id", requireUUIDParam("id"), func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		gogoForms.DefaultHandler(c, forms.EditCommentFormNew(posts, dbUser, c.Param("id")))
	})

	controlsForms.POST("/prompt_post", func(c *gin.Context) {
		dbUser := auth.GetUserData(c).DBUser

		directConnections, err := posts.DirectConnections(c, dbUser)

		if err != nil {
			panic(err)
		}

		gogoForms.DefaultHandler(c, forms.PostPromptFormNew(posts, dbUser, directConnections))
	})
}

// translateKinds maps the kind in a translation URL to the source kind.
var translateKinds = map[string]core.TranslationSourceKind{
	"post":     core.TranslationSourceKindPost,
	"rss_item": core.TranslationSourceKindRSSItem,
}

// mountTranslateRoutes registers the htmx fragments of the translation
// control: the translation of a post or an RSS item, and its original, each
// replacing the slot the page rendered. They answer logged-in htmx requests
// only; every page renders the original itself.
func mountTranslateRoutes(d *Deps, r *gin.RouterGroup) {
	tr := d.Services.Translations

	// serve renders the slot the service answers for the mode.
	serve := func(routeMode translations.SlotMode) gin.HandlerFunc {
		return func(c *gin.Context) {
			mode := routeMode

			kind, ok := translateKinds[c.Param("kind")]
			if !ok || c.GetHeader("HX-Request") != "true" {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			view := c.Query("view")
			if view != "home" && view != "single" {
				view = "feed"
			}

			if mode == translations.SlotOriginal && c.Query("auto") == "1" {
				mode = translations.SlotAuto
			}

			id := c.Param("id")

			res, err := tr.Slot(c, auth.GetUserData(c).DBUser, kind, id, mode)
			if err != nil {
				ginhelpers.HTMLError(c, err)
				return
			}

			slot := map[string]any{
				"Kind": c.Param("kind"), "ID": id, "View": view, "Body": res.Body, "Lang": res.Lang,
				"CanTranslate": res.CanTranslate, "Message": res.Message,
			}

			if res.Result != nil {
				slot["Translated"] = true
				slot["Subject"] = res.Result.Subject
				slot["Body"] = res.Result.Body
				slot["Language"] = forms.LanguageName(res.Result.SourceLang)
				slot["Provider"] = res.Result.Provider
			}

			c.HTML(http.StatusOK, "translation.slot", slot)
		}
	}

	r.GET("/controls/translate/:kind/:id", auth.EnforceAuth, requireUUIDParam("id"), serve(translations.SlotTranslate))

	// the original, which is also where a page lands when it asks what to
	// show: with auto, a language the reader always translates comes back
	// translated.
	r.GET("/controls/translate/:kind/:id/original", auth.EnforceAuth, requireUUIDParam("id"), serve(translations.SlotOriginal))
}
