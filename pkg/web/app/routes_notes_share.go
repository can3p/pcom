package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
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

// shareIgnored are the capture fields a share sheet never sends; the share
// route drops them, so a share is only its text, link and files.
var shareIgnored = []string{web.CaptureFieldTags, web.CaptureFieldSource, web.CaptureFieldCapturedAt, web.CaptureFieldKind}

// mountNoteShareRoutes registers the share target of the installed app: what
// another app shares into pcom (a passage, a link, a photo, a recording)
// becomes a note of the user.
//
// The route takes no CSRF token, because a share target can't send one: the
// phone posts the share sheet's form to the manifest's action. Besides it,
// only POST /subscriptions/:manage_token/unsubscribe/:subscription_id (the
// one-click unsubscribe, authorized by the token in its URL) takes none.
// Leaving the check out is safe here because of two things together. A
// cross-site POST carries no session cookie (it is SameSite=Lax), so a forged
// share lands on the login page like any anonymous request. And the capture
// page's service worker, which sees the share first, sends it on to this
// route and only queues it (to upload later with the session's token) when
// the network fails, so it never turns a forged share into a note. The most a
// share can do is add a note to the sharing user's own notes, which nothing
// else reads before the user does. Every other POST keeps the check.
//
// A share is its title, text and link (shareText) and its files: the tags,
// source and captured_at a capture may carry are ignored. The quote-source
// rule applies as to any shared capture.
func mountNoteShareRoutes(d *Deps, controls *gin.RouterGroup) {
	controls.POST("/notes/share", shareNote(d.Services.Notes))
}

// shareNote saves a share as a note of the session's user and sends the
// phone to the capture page, which shows it as the last capture.
func shareNote(svc *notes.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := parseShareForm(c); err != nil {
			c.String(http.StatusBadRequest, actionMessage(err))
			return
		}

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

		c.Redirect(http.StatusSeeOther, links.Link("notes_new")+"?"+url.Values{web.CaptureSharedParam: {note.ID}}.Encode())
	}
}

// parseShareForm parses the share's form as web.BindCapture does and drops
// the fields of shareIgnored, before web.BindCapture reads the parsed form.
func parseShareForm(c *gin.Context) error {
	r := c.Request

	_, err := c.MultipartForm()
	if errors.Is(err, http.ErrNotMultipart) {
		err = r.ParseForm()
	}

	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return err
		}

		return service.Invalid("body", "The share is not a form.")
	}

	for _, f := range shareIgnored {
		r.Form.Del(f)
		r.PostForm.Del(f)

		if r.MultipartForm != nil {
			delete(r.MultipartForm.Value, f)
		}
	}

	return nil
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
