package formhelpers

import (
	"encoding/json"
	"net/http"

	"github.com/can3p/gogo/forms"
	"github.com/gin-gonic/gin"
)

func ReplaceHistory(action forms.FormSaveAction, url string) forms.FormSaveAction {
	return func(c *gin.Context, f forms.Form) {
		c.Header("HX-Replace-Url", url)
		action(c, f)
	}
}

func Retarget(action forms.FormSaveAction, value string) forms.FormSaveAction {
	return func(c *gin.Context, f forms.Form) {
		c.Header("HX-Retarget", value)
		action(c, f)
	}
}

func Trigger(action forms.FormSaveAction, events gin.H) forms.FormSaveAction {
	b, _ := json.Marshal(events)

	return func(c *gin.Context, f forms.Form) {
		c.Header("HX-Trigger", string(b))
		action(c, f)
	}
}

func NoContent() forms.FormSaveAction {
	return func(c *gin.Context, f forms.Form) {
		c.Status(http.StatusNoContent)
	}
}

func SuccessBadge(msg string) forms.FormSaveAction {
	return Trigger(
		NoContent(),
		gin.H{"operation:success": gin.H{"explanation": msg}})
}
