package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/gin-gonic/gin"
)

// jsonAction is the shape of a JSON action: bind the body into T, run fn for
// the logged-in user and answer with reportSuccess. A body that doesn't bind
// is reported as "Bad input: ...", and an error from fn as actionMessage
// words it. fn is usually one service call.
func jsonAction[T any](_ *Deps, fn func(c *gin.Context, u *model.User, in T) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in T

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := fn(c, auth.GetUserData(c).DBUser, in); err != nil {
			reportError(c, actionMessage(err))
			return
		}

		reportSuccess(c)
	}
}

// actionMessage is the text a JSON action shows for err. Every failed action
// answers 400, whatever the error, so the text is all that differs. A
// ValidationError or a userError is shown as is, and so is any other error,
// as the actions did before the services existed.
func actionMessage(err error) string {
	var invalid *service.ValidationError

	switch {
	case errors.As(err, &invalid):
		return invalid.Message
	case errors.Is(err, service.ErrForbidden):
		return "Operation not allowed"
	case errors.Is(err, service.ErrNotFound):
		return "Not found"
	case errors.Is(err, service.ErrNeedsLogin):
		return "Please log in"
	}

	return err.Error()
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
