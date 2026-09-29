package app

import (
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
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

	r.POST("/create_share", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		var input struct {
			PostID string `json:"postId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		post, err := core.Posts(
			core.PostWhere.ID.EQ(input.PostID),
			qm.Load(core.PostRels.User),
		).One(c, db)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		// drafts are not visible
		if post.PublishedAt.IsZero() {
			reportError(c, "Cannot share a link for draft")
			return
		}

		author := post.R.User

		connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		if connectionRadius.IsUnrelated() {
			reportError(c, "Operation not allowed")
			return
		}

		capabilities := postops.GetPostCapabilities(connectionRadius)

		if !capabilities.CanShare {
			reportError(c, "Operation not allowed")
			return
		}

		shareID, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		share := &core.PostShare{
			ID:     shareID.String(),
			PostID: post.ID,
		}

		err = share.Upsert(c, db, false, []string{core.PostShareColumns.PostID}, boil.Infer(), boil.Infer())

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/delete_share", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		var input struct {
			PostID string `json:"postId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		post, err := core.Posts(
			core.PostWhere.ID.EQ(input.PostID),
			qm.Load(core.PostRels.User),
		).One(c, db)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		author := post.R.User

		connectionRadius, err := userops.GetConnectionRadius(c, db, userData.DBUser.ID, author.ID)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		if connectionRadius.IsUnrelated() {
			reportError(c, "Operation not allowed")
			return
		}

		capabilities := postops.GetPostCapabilities(connectionRadius)

		if !capabilities.CanShare {
			reportError(c, "Operation not allowed")
			return
		}

		_, err = core.PostShares(
			core.PostShareWhere.PostID.EQ(input.PostID),
		).DeleteAll(c, db)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
