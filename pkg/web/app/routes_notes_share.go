package app

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// The fields of the share target that site.webmanifest declares; the files
// come as web.CaptureFieldImage and web.CaptureFieldAudio.
const (
	shareFieldTitle = "title"
	shareFieldText  = "text"
	shareFieldURL   = "url"
)

// mountNoteShareRoutes registers the share target of the installed app: what
// another app shares into pcom (a passage, a link, a photo, a recording)
// becomes a note of the user.
//
// The route takes no CSRF token, because a share target can't send one: the
// phone posts the share sheet's form to the manifest's action. Leaving the
// check out is safe here: a cross-site POST carries no session cookie (it is
// SameSite=Lax), so a forged share lands on the login page like any anonymous
// request, and the most a share can do is add a note to the sharing user's
// own notes, which nothing else reads before the user does. Every other POST
// keeps the check.
//
// The capture page's service worker answers a share itself when it controls
// the page (it queues the capture like the page's own, so it works offline);
// this route takes the shares it doesn't, such as one made before the page
// was ever opened.
func mountNoteShareRoutes(d *Deps, controls *gin.RouterGroup) {
	svc := d.Services.Notes

	controls.POST("/notes/share", func(c *gin.Context) {
		in, err := web.BindCapture(c)
		if err != nil {
			c.String(http.StatusBadRequest, actionMessage(err))
			return
		}

		in.Text = shareText(c.PostForm(shareFieldTitle), c.PostForm(shareFieldText), c.PostForm(shareFieldURL))
		in.How = capture.Shared

		note, err := svc.Capture(c, auth.GetUserData(c).DBUser, in)
		if err != nil {
			c.String(http.StatusBadRequest, actionMessage(err))
			return
		}

		// the capture page shows the note as the last capture
		c.Redirect(http.StatusSeeOther, web.NoteCapturePath+"?"+url.Values{web.CaptureSharedParam: {note.ID}}.Encode())
	})
}

// shareText is the text of a share: its title, text and link, one per line,
// leaving out a part another one already holds (apps often repeat the link in
// the text). A link shared alone stays a bare link. A multipart form sends
// line breaks as CRLF; the text keeps LF, as typed text does. notes_sw.js
// joins a share the same way.
func shareText(parts ...string) string {
	for i := range parts {
		parts[i] = strings.TrimSpace(strings.ReplaceAll(parts[i], "\r\n", "\n"))
	}

	var kept []string

	for i, p := range parts {
		if p == "" {
			continue
		}

		repeated := false

		for j, o := range parts {
			if j != i && strings.Contains(o, p) && (o != p || j < i) {
				repeated = true
				break
			}
		}

		if !repeated {
			kept = append(kept, p)
		}
	}

	return strings.Join(kept, "\n")
}
