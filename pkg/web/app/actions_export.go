package app

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
	"github.com/can3p/pcom/pkg/postops"
	"github.com/gin-gonic/gin"
)

// mountExportActions registers the blog export and import actions.
func mountExportActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB
	mediaStorage := d.MediaStorage

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
