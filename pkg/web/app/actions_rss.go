package app

import (
	"database/sql"
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/feedops"
)

// mountRSSActions registers the RSS subscription actions.
func mountRSSActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/remove_rss_subscription", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		var input struct {
			SubscriptionID string `json:"id"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if input.SubscriptionID == "" {
			reportError(c, "No subscription found")
			return
		}

		// in case feedops make more than one query at some point
		err := transact.Transact(db, func(tx *sql.Tx) error {
			return feedops.UnsubscribeFromFeed(c, tx, userData.DBUser.ID, input.SubscriptionID)
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/dissmiss_rss_item", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		var input struct {
			SubscriptionItemID string `json:"id"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if input.SubscriptionItemID == "" {
			reportError(c, "No item found")
			return
		}

		feedItem, err := core.UserFeedItems(
			core.UserFeedItemWhere.ID.EQ(input.SubscriptionItemID),
			core.UserFeedItemWhere.UserID.EQ(userData.DBUser.ID),
		).One(c, db)

		if err != nil {
			reportError(c, err.Error())
			return
		}

		// in case feedops make more than one query at some point
		err = transact.Transact(db, func(tx *sql.Tx) error {
			feedItem.IsDismissed = true
			_, err := feedItem.Update(c, tx, boil.Infer())

			return err
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
