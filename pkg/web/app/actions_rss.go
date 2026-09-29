package app

import (
	"database/sql"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/feedops"
)

// mountRSSActions registers the RSS subscription actions.
func mountRSSActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/remove_rss_subscription", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		SubscriptionID string `json:"id"`
	}) error {
		if input.SubscriptionID == "" {
			return userError(("No subscription found"))
		}

		// in case feedops make more than one query at some point
		err := transact.Transact(db, func(tx *sql.Tx) error {
			return feedops.UnsubscribeFromFeed(c, tx, dbUser.ID, input.SubscriptionID)
		})

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))

	r.POST("/dissmiss_rss_item", jsonAction(d, func(c *gin.Context, dbUser *core.User, input struct {
		SubscriptionItemID string `json:"id"`
	}) error {
		if input.SubscriptionItemID == "" {
			return userError(("No item found"))
		}

		feedItem, err := core.UserFeedItems(
			core.UserFeedItemWhere.ID.EQ(input.SubscriptionItemID),
			core.UserFeedItemWhere.UserID.EQ(dbUser.ID),
		).One(c, db)

		if err != nil {
			return err
		}

		// in case feedops make more than one query at some point
		err = transact.Transact(db, func(tx *sql.Tx) error {
			feedItem.IsDismissed = true
			_, err := feedItem.Update(c, tx, boil.Infer())

			return err
		})

		if err != nil {
			return userError(fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		return nil
	}))
}
