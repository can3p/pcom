package ginhelpers

import (
	"errors"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
)

// The page errors are the service errors, so a page builder and a service
// can return either. New code returns the service ones.
var (
	ErrNotFound   = service.ErrNotFound
	ErrNeedsLogin = service.ErrNeedsLogin
	ErrForbidden  = service.ErrForbidden
	ErrBadRequest = errors.New("invalid input")
)

// Status is the HTTP status for an error a page or an API call failed with.
// It is the one mapping from service errors to statuses; ErrNeedsLogin is
// not a status, and HTML handles it before asking.
func Status(err error) int {
	var invalid *service.ValidationError

	switch {
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, service.ErrNeedsLogin):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrBadRequest), errors.As(err, &invalid):
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}

func HTML[T any](c *gin.Context, templateName string, result mo.Result[T]) {
	if result.IsOk() {
		c.HTML(http.StatusOK, templateName, result.MustGet())
		return
	}

	HTMLError(c, result.Error())
}

// HTMLError answers a page request that failed with err: a redirect to the
// login page for ErrNeedsLogin, otherwise Status(err).
func HTMLError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNeedsLogin) {
		auth.RedirectToLogin(c)
		c.Abort()
		return
	}

	httpCode := Status(err)

	if util.InCluster() {
		c.Status(httpCode)
		return
	}

	c.String(httpCode, err.Error())
}

func API[T any](c *gin.Context, result mo.Result[T]) {
	if result.IsOk() {
		c.JSON(http.StatusOK, gin.H{
			"data": result.MustGet(),
		})
		return
	}

	httpCode := Status(result.Error())

	if util.InCluster() {
		c.Status(httpCode)
		return
	}

	c.JSON(httpCode, gin.H{
		"errors": []string{result.Error().Error()},
	})
}
