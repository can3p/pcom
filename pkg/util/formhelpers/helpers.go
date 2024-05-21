package formhelpers

import (
	"github.com/can3p/gogo/forms"
	"github.com/gin-gonic/gin"
)

func ReplaceHistory(action forms.FormSaveAction, url string) forms.FormSaveAction {
	return func(c *gin.Context, f forms.Form) {
		c.Header("HX-Replace-Url", url)
		action(c, f)
	}
}
