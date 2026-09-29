package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// mountShareActions registers the post sharing actions.
func mountShareActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/create_share", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		PostID string `json:"postId"`
	}) error {
		post, err := core.Posts(
			core.PostWhere.ID.EQ(input.PostID),
			qm.Load(core.PostRels.User),
		).One(c, db)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		// drafts are not visible
		if post.PublishedAt.IsZero() {
			return userError(("Cannot share a link for draft"))
		}

		author := post.R.User

		connectionRadius, err := userops.GetConnectionRadius(c, db, dbUser.ID, author.ID)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		if connectionRadius.IsUnrelated() {
			return userError(("Operation not allowed"))
		}

		capabilities := postops.GetPostCapabilities(connectionRadius)

		if !capabilities.CanShare {
			return userError(("Operation not allowed"))
		}

		shareID, err := uuid.NewV7()

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		share := &core.PostShare{
			ID:     shareID.String(),
			PostID: post.ID,
		}

		err = share.Upsert(c, db, false, []string{core.PostShareColumns.PostID}, boil.Infer(), boil.Infer())

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/delete_share", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		PostID string `json:"postId"`
	}) error {
		post, err := core.Posts(
			core.PostWhere.ID.EQ(input.PostID),
			qm.Load(core.PostRels.User),
		).One(c, db)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		author := post.R.User

		connectionRadius, err := userops.GetConnectionRadius(c, db, dbUser.ID, author.ID)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		if connectionRadius.IsUnrelated() {
			return userError(("Operation not allowed"))
		}

		capabilities := postops.GetPostCapabilities(connectionRadius)

		if !capabilities.CanShare {
			return userError(("Operation not allowed"))
		}

		_, err = core.PostShares(
			core.PostShareWhere.PostID.EQ(input.PostID),
		).DeleteAll(c, db)

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
