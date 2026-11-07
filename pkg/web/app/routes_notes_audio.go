package app

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
)

// mountNoteAudioRoutes registers the recording upload, the private serving of
// a note's recording and the transcription state the notes pages poll.
func mountNoteAudioRoutes(d *Deps, controls, actions *gin.RouterGroup) {
	svc := d.Services.Notes

	// A multipart form: the recording in "file"; "note_id" gives an existing
	// note the recording, otherwise a new note is made, captured at
	// "captured_at" (RFC 3339) when the client says so, under the focus.
	actions.POST("/note_audio", func(c *gin.Context) {
		user := auth.GetUserData(c).DBUser

		file, err := c.FormFile("file")
		if err != nil {
			reportError(c, "Choose a recording to upload.")
			return
		}

		f, err := file.Open()
		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		defer func() { _ = f.Close() }()

		var note *model.Note

		if id := c.PostForm("note_id"); id != "" {
			note, err = svc.AttachAudio(c, user, id, f, file.Header.Get("Content-Type"))
		} else {
			in := notes.Input{ApplyFocus: true}

			if at := c.PostForm("captured_at"); at != "" {
				if in.CapturedAt, err = time.Parse(time.RFC3339, at); err != nil {
					reportError(c, "Bad input: captured_at is not a time.")
					return
				}
			}

			note, err = svc.CreateFromAudio(c, user, f, file.Header.Get("Content-Type"), in)
		}

		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"note_id": note.ID})
	})

	// The recording streams with Range support, for the <audio> element to
	// seek, and is never cached by anything but the owner's browser.
	controls.GET("/notes/:id/audio", requireUUIDParam("id"), func(c *gin.Context) {
		rec, err := svc.Recording(c, auth.GetUserData(c).DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.Header("Cache-Control", "private")
		c.Header("Content-Type", rec.ContentType)
		http.ServeContent(c.Writer, c.Request, "", rec.ModTime, bytes.NewReader(rec.Bytes))
	})

	controls.GET("/notes/:id/status", requireUUIDParam("id"), func(c *gin.Context) {
		note, err := svc.Get(c, auth.GetUserData(c).DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		c.HTML(http.StatusOK, "partial--note-status.html", gin.H{"Note": note})
	})
}
