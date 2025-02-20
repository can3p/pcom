package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

func setupActions(r *gin.RouterGroup, db *sqlx.DB, mediaStorage server.MediaStorage) {
	r.POST("/remove_from_whitelist", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			UserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DropConnectionGrant(c, db, dbUser.ID, input.UserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/create_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.EstablishConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/drop_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DropConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/request_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID  string `json:"userId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.RequestMediation(c, db, dbUser.ID, input.TargetUserID, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/revoke_mediation_request", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.RevokeMediationRequest(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/dismiss_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID     string `json:"requestId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionDismissed, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/sign_mediation", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID     string `json:"requestId"`
			MediationNote string `json:"mediation_note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideForwardMediationRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionMediationDecisionSigned, input.MediationNote); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/reject_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID string `json:"requestId"`
			Note      string `json:"note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideConnectionRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionRequestDecisionDismissed, input.Note); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/accept_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			RequestID string `json:"requestId"`
			Note      string `json:"note"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DecideConnectionRequest(c, db, dbUser.ID, input.RequestID, core.ConnectionRequestDecisionApproved, input.Note); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/delete_draft", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			PostID string `json:"postId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		err := transact.Transact(db, func(tx *sql.Tx) error {
			post, err := core.Posts(
				core.PostWhere.ID.EQ(input.PostID),
				core.PostWhere.UserID.EQ(dbUser.ID),
				core.PostWhere.PublishedAt.IsNull(),
				qm.For("Update"),
			).One(c, db)

			if err != nil {
				return err
			}

			return postops.DeletePost(c, tx, post.ID)
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/generate_api_key", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		id, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		newApiKey, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
		}

		err = transact.Transact(db, func(tx *sql.Tx) error {
			record := core.UserAPIKey{
				ID:     id.String(),
				APIKey: newApiKey.String(),
				UserID: dbUser.ID,
			}

			// no key rotation for now
			// feel free to implement/change
			return record.Upsert(c, tx, false, []string{core.UserAPIKeyColumns.UserID}, boil.Infer(), boil.Infer())
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/dismiss_prompt", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			PromptID string `json:"promptId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		prompt, err := core.PostPrompts(
			core.PostPromptWhere.RecipientID.EQ(dbUser.ID),
			core.PostPromptWhere.ID.EQ(input.PromptID),
		).One(c, db)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		prompt.DismissedAt = null.TimeFrom(time.Now())

		if _, err := prompt.Update(c, db, boil.Infer()); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

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

	r.POST("/upload_media", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		res := web.ApiUploadImage(c, db, userData.DBUser, mediaStorage)

		if res.IsError() {
			reportError(c, fmt.Sprintf("Operation Failed: %s", res.Error()))
			return
		}

		resp := res.MustGet()

		c.JSON(http.StatusOK, gin.H{
			"uploaded_url": resp.ImageID,
		})
	})

	// XXX: this endpoint should be rebuilt to generate archive asyncronously
	r.POST("/settings/export", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		user := userData.User.DBUser

		b, err := postops.SerializeBlog(c, db, mediaStorage, user.ID)

		if err != nil {
			panic(err)
		}

		fname := fmt.Sprintf("export_%s_%s.zip", user.Username, time.Now().Format(time.RFC3339))
		contentLength := int64(len(b))
		contentType := "application/zip"

		reader := bytes.NewReader(b)

		extraHeaders := map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fname),
		}

		c.DataFromReader(http.StatusOK, contentLength, contentType, reader, extraHeaders)
	})

	// XXX: this endpoint should be rebuilt to generate archive asyncronously
	r.POST("/settings/import", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		user := userData.User.DBUser

		fh, err := c.FormFile("file")

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		f, err := fh.Open()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		defer func() {
			if err := f.Close(); err != nil {
				log.Printf("Error closing file: %v", err)
			}
		}()

		b, err := io.ReadAll(f)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		posts, images, err := postops.DeserializeArchive(b)

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		var stats *postops.InjectStats

		err = transact.Transact(db, func(tx *sql.Tx) error {
			stats, err = postops.InjectPostsInDB(c, tx, mediaStorage, user.ID, posts, images)

			return err
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		b, err = json.Marshal(stats)

		if err != nil {
			panic(err)
		}

		c.String(http.StatusOK, string(b))
	})

}

func reportError(c *gin.Context, s string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"explanation": s,
	})
}

func reportSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{})
}
