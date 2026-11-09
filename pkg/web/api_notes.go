package web

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/can3p/gogo/util/ginhelpers"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

// The form fields of a capture (BindCapture).
const (
	CaptureFieldText       = "text"
	CaptureFieldTags       = "tags"
	CaptureFieldCapturedAt = "captured_at"
	CaptureFieldKind       = "kind"
	CaptureFieldSource     = "source"
	CaptureFieldImage      = "image"
	CaptureFieldAudio      = "audio"
)

// captureKinds maps the kind field of a capture to how its text arrived.
var captureKinds = map[string]capture.How{
	"":      capture.Typed,
	"note":  capture.Typed,
	"quote": capture.QuoteShortcut,
	"share": capture.Shared,
}

// BindCapture reads a capture from a multipart (or url-encoded) form:
// text, tags (repeated, or comma-separated), captured_at (RFC 3339), kind
// (note, quote or share), source (a book title), any number of image files
// and at most one audio file. A malformed field is a service.Invalid for it;
// a body over its cap is the *http.MaxBytesError bodyLimits causes.
func BindCapture(c *gin.Context) (notes.CaptureInput, error) {
	var in notes.CaptureInput

	_, err := c.MultipartForm()
	if errors.Is(err, http.ErrNotMultipart) {
		err = c.Request.ParseForm()
	}

	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return in, err
		}

		return in, service.Invalid("body", "The capture is not a form.")
	}

	// a multipart form sends line breaks as CRLF; a note keeps LF, as typed
	in.Text = strings.ReplaceAll(c.PostForm(CaptureFieldText), "\r\n", "\n")

	for _, t := range c.PostFormArray(CaptureFieldTags) {
		for tag := range strings.SplitSeq(t, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				in.Tags = append(in.Tags, tag)
			}
		}
	}

	if at := strings.TrimSpace(c.PostForm(CaptureFieldCapturedAt)); at != "" {
		if in.CapturedAt, err = time.Parse(time.RFC3339, at); err != nil {
			return in, service.Invalid(CaptureFieldCapturedAt, "captured_at is not an RFC 3339 time, such as 2026-11-07T09:30:00+01:00.")
		}
	}

	how, ok := captureKinds[strings.TrimSpace(c.PostForm(CaptureFieldKind))]
	if !ok {
		return in, service.Invalid(CaptureFieldKind, "kind is note, quote or share.")
	}

	in.How = how

	in.Source = c.PostForm(CaptureFieldSource)

	var files map[string][]*multipart.FileHeader
	if c.Request.MultipartForm != nil {
		files = c.Request.MultipartForm.File
	}

	for _, fh := range files[CaptureFieldImage] {
		u, err := readUpload(fh)
		if err != nil {
			return in, err
		}

		in.Images = append(in.Images, u)
	}

	switch audio := files[CaptureFieldAudio]; len(audio) {
	case 0:
	case 1:
		u, err := readUpload(audio[0])
		if err != nil {
			return in, err
		}

		in.Audio = &u
	default:
		return in, service.Invalid(CaptureFieldAudio, "Send at most one recording.")
	}

	return in, nil
}

// readUpload reads a file of a form; the body cap bounds its size.
func readUpload(fh *multipart.FileHeader) (notes.Upload, error) {
	f, err := fh.Open()
	if err != nil {
		return notes.Upload{}, err
	}

	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(f)
	if err != nil {
		return notes.Upload{}, err
	}

	return notes.Upload{Body: bytes.NewReader(b), ContentType: fh.Header.Get("Content-Type")}, nil
}

// ApiNote is a note as the API returns it.
type ApiNote struct {
	ID                  string                        `json:"id"`
	Body                string                        `json:"body"`
	Tags                []string                      `json:"tags"`
	Source              string                        `json:"source,omitempty"`
	TranscriptionStatus model.NoteTranscriptionStatus `json:"transcription_status"`
	CapturedAt          time.Time                     `json:"captured_at"`
	Images              []string                      `json:"images"`
}

// ApiGetNotesResponse is a page of notes; Cursor is empty on the last one.
type ApiGetNotesResponse struct {
	Notes  []*ApiNote `json:"notes"`
	Cursor string     `json:"cursor"`
}

func apiNote(n *model.Note, site links.Site) *ApiNote {
	out := &ApiNote{
		ID:                  n.ID,
		Body:                n.Body,
		Tags:                lo.Union(n.Tags, n.TextTags),
		TranscriptionStatus: n.TranscriptionStatus,
		CapturedAt:          n.CapturedAt.UTC(),
		Images:              []string{},
	}

	if n.Source != nil {
		out.Source = n.Source.Title
	}

	for _, img := range n.Images {
		if img.Upload != nil {
			out.Images = append(out.Images, site.Abs("uploaded_media", img.Upload.UploadedFname))
		}
	}

	return out
}

// ApiCreateNote is POST /api/v1/notes: a capture from a phone (BindCapture).
func ApiCreateNote(c *gin.Context, svc *notes.Service, actor *model.User, site links.Site) mo.Result[*ApiNote] {
	in, err := BindCapture(c)
	if err != nil {
		return mo.Err[*ApiNote](err)
	}

	n, err := svc.Capture(c, actor, in)
	if err != nil {
		return mo.Err[*ApiNote](err)
	}

	return mo.Ok(apiNote(n, site))
}

// ApiGetNotes is GET /api/v1/notes: a page of the actor's notes, newest
// capture first, filtered like the notes page.
func ApiGetNotes(c *gin.Context, svc *notes.Service, actor *model.User, site links.Site) mo.Result[*ApiGetNotesResponse] {
	var form struct {
		Tags   []string `form:"tag"`
		Not    []string `form:"not"`
		Query  string   `form:"q"`
		Cursor string   `form:"cursor"`
		Limit  int      `form:"limit"`
	}

	if err := c.ShouldBindQuery(&form); err != nil {
		return mo.Err[*ApiGetNotesResponse](service.Invalid("query", err.Error()))
	}

	page, err := svc.ListUpTo(c, actor, notes.Filter{
		Tags: form.Tags, NotTags: form.Not, Query: form.Query, Cursor: form.Cursor,
	}, form.Limit)
	if err != nil {
		return mo.Err[*ApiGetNotesResponse](err)
	}

	return mo.Ok(&ApiGetNotesResponse{
		Notes:  lo.Map(page.Entries, func(e notes.Entry, _ int) *ApiNote { return apiNote(e.Note, site) }),
		Cursor: page.Next,
	})
}

// ApiNotesRespond answers a notes API call. Unlike ginhelpers.API it always
// says why input was refused, since the caller is a phone Shortcut that shows
// the answer to its user: 400 with {"errors": [message], "field": field}, and
// 413 for a body over its cap.
func ApiNotesRespond[T any](c *gin.Context, result mo.Result[T]) {
	err := result.Error()

	var invalid *service.ValidationError

	var tooLarge *http.MaxBytesError

	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"errors": []string{invalid.Message}, "field": invalid.Field})
	case errors.As(err, &tooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"errors": []string{"The capture is too large."}})
	default:
		ginhelpers.API(c, result)
	}
}
