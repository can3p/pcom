package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/gin-gonic/gin"
)

// sendZip answers with a zip archive as a download.
func sendZip(c *gin.Context, username string, archive []byte) {
	fname := fmt.Sprintf("export_%s_%s.zip", username, time.Now().Format(time.RFC3339))

	c.DataFromReader(http.StatusOK, int64(len(archive)), "application/zip", bytes.NewReader(archive), map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fname),
	})
}

// mountExportActions registers the blog export and import actions.
func mountExportActions(d *Deps, r *gin.RouterGroup) {
	posts := d.Services.Posts

	// XXX: this endpoint should be rebuilt to generate archive asyncronously
	r.POST("/settings/export", func(c *gin.Context) {
		user := auth.GetUserData(c).User.DBUser

		b, err := posts.ExportBlog(c, user)

		if err != nil {
			panic(err)
		}

		sendZip(c, user.Username, b)
	})

	// XXX: this endpoint should be rebuilt to generate archive asyncronously
	r.POST("/settings/import", func(c *gin.Context) {
		user := auth.GetUserData(c).User.DBUser

		b, err := readUpload(c, "file")

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		stats, err := posts.Import(c, user, b)

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

// readUpload returns the content of the uploaded form file.
func readUpload(c *gin.Context, field string) ([]byte, error) {
	fh, err := c.FormFile(field)
	if err != nil {
		return nil, err
	}

	f, err := fh.Open()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()

	return io.ReadAll(f)
}
