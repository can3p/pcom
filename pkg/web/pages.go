package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/service/registry"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/samber/mo"
)

type BasePage struct {
	ProjectName string
	Name        string
	User        *auth.UserData
	StyleNonce  *string
	ScriptNonce *string
	RSSFeed     string
}

func getBasePage(c *gin.Context, name string, userData *auth.UserData) *BasePage {
	return &BasePage{
		Name:        name,
		User:        userData,
		ProjectName: "pcom",
		StyleNonce:  csp.GetStyleNonce(c),
		ScriptNonce: csp.GetScriptNonce(c),
	}
}

func Index(c *gin.Context, userData *auth.UserData) *BasePage {
	return getBasePage(c, "Social network for private groups", userData)
}

// SinglePost is the visibility check of /posts/:id/zip in routes_posts.go,
// which RS.L3 leaves as it is while RS.L1 moves the post page to the reading
// service. It goes, with this file's allowlist entry, once that route calls
// the reading service itself.
func SinglePost(c *gin.Context, db *sqlx.DB, userData *auth.UserData, postID string, editPreview bool) mo.Result[*SinglePostPage] {
	post, err := registry.New(db, registry.Deps{}).Reading.Post(c, userData.DBUser, postID, editPreview)
	if err != nil {
		return mo.Err[*SinglePostPage](err)
	}

	return mo.Ok(PostPage(c, userData, post))
}
