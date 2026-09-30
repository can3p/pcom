package web

import (
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-gonic/gin"
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
