package app

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"

	"encoding/json"
	"io"

	"github.com/can3p/gogo/util/transact"
)

// mountSettingsActions registers the account settings actions.
func mountSettingsActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB
	mediaStorage := d.MediaStorage

	r.POST("/generate_api_key", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		id, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		newApiKey, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
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

	// creates the private feed token or replaces it; the old feed URL stops working
	r.POST("/regenerate_feed_token", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if _, err := repo.RegenerateFeedToken(c.Request.Context(), db, userData.DBUser.ID); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
