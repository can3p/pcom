package pgsession

import (
	"database/sql"
	"errors"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
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
	DBUser *model.User
}

func GetUser(c *gin.Context) *User {
	v, ok := c.Get(userContextKey.String())

	if !ok {
		return nil
	}

	return v.(*User)
}

func SetUser(c *gin.Context, db *sqlx.DB, userID string) error {
	u, err := repo.New(db).UserByID(c.Request.Context(), userID)
	if errors.Is(err, repo.ErrNotFound) {
		return sql.ErrNoRows
	}

	if err != nil {
		return err
	}

	SetLoadedUser(c, u)

	return nil
}

// SetLoadedUser makes an already loaded user the request's user, as SetUser
// does after loading one.
func SetLoadedUser(c *gin.Context, u *model.User) {
	c.Set(userContextKey.String(), &User{u})
}
