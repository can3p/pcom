package app

import (
	"github.com/can3p/pcom/pkg/model"
	"github.com/gin-gonic/gin"
)

type shareInput struct {
	PostID string `json:"postId"`
}

// mountShareActions registers the post sharing actions.
func mountShareActions(d *Deps, r *gin.RouterGroup) {
	shares := d.Services.Shares

	r.POST("/create_share", jsonAction(d, func(c *gin.Context, u *model.User, in shareInput) error {
		return shares.Create(c, u, in.PostID)
	}))

	r.POST("/delete_share", jsonAction(d, func(c *gin.Context, u *model.User, in shareInput) error {
		return shares.Delete(c, u, in.PostID)
	}))
}
