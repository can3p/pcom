package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
)

// jsonAction is the shape of a JSON action: bind the body into T, run fn for
// the logged-in user and answer with reportSuccess. A body that doesn't bind
// is reported as "Bad input: ...", and an error from fn is reported with its
// message as is, so fn words the error the way the user should see it.
func jsonAction[T any](d *Deps, fn func(c *gin.Context, u *core.User, in T) error) gin.HandlerFunc {
	_ = d // RS hands the services to fn through d

	return func(c *gin.Context) {
		var in T

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := fn(c, auth.GetUserData(c).DBUser, in); err != nil {
			reportError(c, err.Error())
			return
		}

		reportSuccess(c)
	}
}

func reportError(c *gin.Context, s string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"explanation": s,
	})
}

func reportSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{})
}

// userError builds an error whose text is shown to the user as is, so it
// reads as a sentence rather than as a Go error string.
func userError(s string) error {
	return errors.New(s)
}
