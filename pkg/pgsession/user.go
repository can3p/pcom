package pgsession

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type contextKey string

func (c contextKey) String() string {
	return "user context key " + string(c)
}

const (
	userContextKey = contextKey("user")
)

type User struct {
	DBUser *core.User
}

func GetUser(c *gin.Context) *User {
	v, ok := c.Get(userContextKey.String())

	if !ok {
		return nil
	}

	return v.(*User)
}

func SetUser(c *gin.Context, db *sqlx.DB, userID string) error {
	u, err := core.FindUser(c.Request.Context(), db, userID)

	if err != nil {
		return err
	}

	c.Set(userContextKey.String(), &User{u})

	return nil
}
