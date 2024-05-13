package ginhelpers

import (
	"net/http"

	"github.com/can3p/pcom/pkg/util"
	"github.com/friendsofgo/errors"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
)

var ErrNotFound = errors.Errorf("not found")
var ErrForbidden = errors.Errorf("forbidden")
var ErrBadRequest = errors.Errorf("invalid input")

func HTML[T any](c *gin.Context, templateName string, result mo.Result[T]) {
	if result.IsOk() {
		c.HTML(http.StatusOK, templateName, result.MustGet())
		return
	}

	var httpCode int = http.StatusInternalServerError

	switch result.Error() {
	case ErrNotFound:
		httpCode = http.StatusNotFound
	case ErrForbidden:
		httpCode = http.StatusForbidden
	case ErrBadRequest:
		httpCode = http.StatusBadRequest
	}

	if util.InCluster() {
		c.Status(httpCode)
		return
	}

	c.String(httpCode, result.Error().Error())
}
