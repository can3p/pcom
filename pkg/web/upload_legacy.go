package web

import (
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/samber/mo"
)

// ApiUploadImage is ApiUploadImageWith for the media action, which still
// holds the database (RS.L6 gives it a service and deletes this file).
func ApiUploadImage(c *gin.Context, db *sqlx.DB, dbUser *core.User, mediaStorage server.MediaStorage) mo.Result[*ApiUploadImageResponse] {
	return ApiUploadImageWith(c, posts.New(repo.New(db), nil, mediaStorage), dbUser)
}
