package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// ProjectName is the site name shown in page and feed titles.
const ProjectName = "pcom"

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
		ProjectName: ProjectName,
		StyleNonce:  csp.GetStyleNonce(c),
		ScriptNonce: csp.GetScriptNonce(c),
	}
}

// Index is / for an anonymous visitor: the public posts, read-only, and the
// site-wide RSS feed.
func Index(c *gin.Context, userData *auth.UserData, posts []*postops.Post) *FeedPage {
	base := getBasePage(c, "Social network for private groups", userData)
	base.RSSFeed = links.Link("public_feed")

	return &FeedPage{
		BasePage: base,
		Items: lo.Map(posts, func(p *postops.Post, _ int) *FeedItem {
			return &FeedItem{Post: p}
		}),
	}
}
