package app

import (
	"database/sql"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"

	"github.com/can3p/gogo/util/transact"
)

// mountPostActions registers the post actions.
func mountPostActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/delete_draft", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		PostID string `json:"postId"`
	}) error {
		err := transact.Transact(db, func(tx *sql.Tx) error {
			post, err := core.Posts(
				core.PostWhere.ID.EQ(input.PostID),
				core.PostWhere.UserID.EQ(dbUser.ID),
				core.PostWhere.PublishedAt.IsNull(),
				qm.For("Update"),
			).One(c, tx)

			if err != nil {
				return err
			}

			return postops.DeletePost(c, tx, post.ID)
		})

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
