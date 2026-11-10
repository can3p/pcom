package app

import (
	"errors"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// notesServiceWorker is where the build of the notes service worker
// (client/js/notes/sw.js) is under the process cwd. It uploads the captures
// the capture page queues in IndexedDB, with retries, and keeps the capture
// page's assets for offline use. It is served from /controls/notes/, which is
// its scope, so it touches no other page; that is also why it is not served
// from /static or the CDN.
var notesServiceWorker = sync.OnceValues(func() (string, error) {
	files, err := readStaticManifest()
	if err != nil {
		return "", err
	}

	path, ok := files["notes_sw.js"]
	if !ok {
		return "", errors.New("no notes_sw.js in the asset manifest")
	}

	return filepath.Join("dist", filepath.FromSlash(path)), nil
})

// mountNoteCaptureRoutes registers the capture page, its service worker and
// the capture action the service worker sends a queued capture to.
func mountNoteCaptureRoutes(d *Deps, controls, actions *gin.RouterGroup) {
	svc := d.Services.Notes

	controls.GET("/notes/new", func(c *gin.Context) {
		// The page runs its own bundle instead of main.js, and htmx doesn't
		// run the scripts of a page it swaps in, so a boosted link loads it
		// whole.
		if c.GetHeader("HX-Request") == "true" {
			c.Header("HX-Redirect", c.Request.URL.RequestURI())
			c.Status(http.StatusOK)

			return
		}

		userData := auth.GetUserData(c)

		ginhelpers.HTML(c, "notes_new.html", web.NoteCapture(c, svc, &userData))
	})

	controls.GET("/notes/sw.js", func(c *gin.Context) {
		path, err := notesServiceWorker()
		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Type", "text/javascript; charset=utf-8")
		c.File(path)
	})

	// A multipart capture (web.BindCapture); the answer is the new note's id
	// and the confirmation its Delete asks.
	actions.POST("/note_capture", func(c *gin.Context) {
		in, err := web.BindCapture(c)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		note, err := svc.Capture(c, auth.GetUserData(c).DBUser, in)
		if err != nil {
			reportError(c, actionMessage(err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"note_id": note.ID, "delete_prompt": web.NoteDeletePrompt(note, false)})
	})
}
