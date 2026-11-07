package app

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/web"
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

	// "Transcribe again as…": sends the recording once more with the language
	// the author picked. The page reloads on success; a refusal the author can
	// act on (limit reached, a transcription running) answers in place, the
	// rest (a foreign or missing note) as every action does.
	actions.POST("/note_retranscribe", func(c *gin.Context) {
		var in struct {
			NoteID   string `json:"note_id"`
			Language string `json:"language"`
		}

		if err := c.BindJSON(&in); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		err := svc.Retranscribe(c, auth.GetUserData(c).DBUser, in.NoteID, in.Language)

		var invalid *service.ValidationError

		switch {
		case err == nil:
			c.Header("HX-Refresh", "true")
			c.Status(http.StatusOK)
		case errors.As(err, &invalid):
			c.HTML(http.StatusOK, "partial--error-message.html", gin.H{"Message": invalid.Message})
		default:
			reportError(c, actionMessage(err))
		}
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

	// The poll of a pending transcription (partial--note-status.html). Once
	// it is over, the answer depends on where the status shows: on the list
	// the whole note is replaced; on the edit page the text box takes the
	// note's new body unless the author changed the text since the page
	// opened ("body" against "base"), in which case the transcript shows to
	// copy.
	controls.GET("/notes/:id/status", requireUUIDParam("id"), func(c *gin.Context) {
		userData := auth.GetUserData(c)

		note, err := svc.Get(c, userData.DBUser, c.Param("id"))
		if err != nil {
			ginhelpers.HTMLError(c, err)
			return
		}

		status := &web.NoteStatus{
			Note: note, View: c.Query("view"), Stack: c.Query("stack"), Suggest: c.Query("suggest") == "1",
			Base: lf(c.Query("base")),
		}

		switch {
		case note.TranscriptionStatus == model.NoteTranscriptionPending:
		case status.View == web.NoteStatusList:
			item, err := web.NoteItemOf(c, svc, userData.DBUser, note, status)
			if err != nil {
				ginhelpers.HTMLError(c, err)
				return
			}

			c.Header("HX-Retarget", "#note-"+note.ID)
			c.Header("HX-Reswap", "outerHTML")
			c.HTML(http.StatusOK, "partial--note-item.html", gin.H{"Item": item, "User": &userData, "Stack": status.Stack})

			return
		case status.View == web.NoteStatusEdit && note.TranscriptionStatus == model.NoteTranscriptionDone:
			body := lf(c.Query("body"))
			if body == status.Base {
				c.HTML(http.StatusOK, "note_transcribed", gin.H{"Status": status, "Tags": web.NoteTagsOf(note), "Again": web.NoteAgainOf(svc, note, true)})
				return
			}

			// what was typed may have been saved before the text came
			status.Typed, status.Transcript = true, web.TranscriptOf(lf(note.Body), body, status.Base)
		}

		c.HTML(http.StatusOK, "partial--note-status.html", status)

		// the transcription is over: the edit page's control comes back, out of band
		if status.View == web.NoteStatusEdit && note.TranscriptionStatus != model.NoteTranscriptionPending {
			c.HTML(http.StatusOK, "note_again_slot", gin.H{"Again": web.NoteAgainOf(svc, note, true), "OOB": true})
		}
	})
}

// lf is s with the line ends a browser may send as CRLF written as LF.
func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
