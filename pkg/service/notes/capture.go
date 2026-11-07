package notes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	mediasvc "github.com/can3p/pcom/pkg/service/media"
)

// ListMax is the most notes one call of ListUpTo returns, as posts.ListMax
// is for posts.
const ListMax = 100

// What the actor reads when a capture is refused.
const (
	msgCaptureEmpty = "Send some text, a photo or a recording."
	msgImageFormat  = "pcom keeps photos in JPEG, PNG or WebP."
	msgImageHEIC    = "pcom can't read HEIC photos: add \"Convert to JPEG\" before sending the photo in the Shortcut."
	msgImageTooBig  = "A photo can be at most %d MiB."
)

// Upload is a file of a capture: its bytes and the client's word for its
// type, which is only a hint.
type Upload struct {
	Body        io.Reader
	ContentType string
}

// CaptureInput is what one capture sends: text, photos and at most one
// recording, any of them alone.
type CaptureInput struct {
	Text string
	// How the text arrived; it decides how capture.Parse stores it.
	How  capture.How
	Tags []string
	// CapturedAt is when the note was taken; the zero value means now.
	CapturedAt time.Time
	// Source is the source to attach, found or created. A quote without one
	// takes QuoteSource.
	Source *SourceInput
	Images []Upload
	Audio  *Upload
}

// Capture saves a note of the actor from a capture: the text parsed by
// capture.Parse, the tags with the focus applied and renewed, the source,
// the photos in the order given and the recording with its transcription,
// all in one transaction; the files land in the storage last, which is not
// part of it. Every file is checked before anything is written.
func (s *Service) Capture(ctx context.Context, actor *model.User, in CaptureInput) (*model.Note, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	parsed := capture.Parse(in.Text, in.How)

	if parsed.Text == "" && len(in.Images) == 0 && in.Audio == nil {
		return nil, service.Invalid("text", msgCaptureEmpty)
	}

	images, err := readImages(in.Images, s.maxUploadSize)
	if err != nil {
		return nil, err
	}

	if len(images) > 0 && s.media == nil {
		return nil, errors.New("notes: no media storage to keep photos in")
	}

	var rec *recording

	if in.Audio != nil {
		if rec, err = s.readRecording(in.Audio.Body, in.Audio.ContentType); err != nil {
			return nil, err
		}
	}

	var noteID string

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		within := s.within(tx)

		var quoteSource *model.NoteSource

		if in.Source == nil && parsed.Kind == capture.Quote {
			if quoteSource, err = within.QuoteSource(ctx, actor); err != nil {
				return err
			}
		}

		n, err := within.Create(ctx, actor, Input{Body: parsed.Text, Tags: in.Tags, Source: in.Source, CapturedAt: in.CapturedAt, ApplyFocus: true})
		if err != nil {
			return err
		}

		noteID = n.ID

		if quoteSource != nil {
			n.SourceID, n.Source = &quoteSource.ID, quoteSource
			if err := tx.SaveNote(ctx, n, "source_id", "updated_at"); err != nil {
				return err
			}
		}

		for i, img := range images {
			fname, err := mediasvc.StoreUpload(ctx, tx, s.media, &actor.ID, nil, bytes.NewReader(img), int64(len(img)))
			if err != nil {
				return err
			}

			uploadID := strings.TrimSuffix(fname, path.Ext(fname))
			if err := tx.CreateNoteImage(ctx, &model.NoteImage{NoteID: n.ID, UploadID: uploadID, Position: i}); err != nil {
				return err
			}
		}

		if rec != nil {
			return s.attach(ctx, tx, n, rec)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, actor, noteID)
}

// readImages reads every photo of a capture and checks that pcom keeps it,
// so that a bad photo refuses the capture before anything is stored.
func readImages(uploads []Upload, maxSize int64) ([][]byte, error) {
	images := make([][]byte, 0, len(uploads))

	for _, u := range uploads {
		b, err := io.ReadAll(io.LimitReader(u.Body, maxSize+1))
		if err != nil {
			return nil, err
		}

		switch {
		case int64(len(b)) > maxSize:
			return nil, service.Invalid("image", fmt.Sprintf(msgImageTooBig, maxSize>>20))
		case isHEIC(b):
			return nil, service.Invalid("image", msgImageHEIC)
		}

		if _, _, err := media.DetectAndValidateImageTypeAndExt(b); errors.Is(err, media.ErrUnsupportedMimeType) {
			return nil, service.Invalid("image", msgImageFormat)
		} else if err != nil {
			return nil, err
		}

		images = append(images, b)
	}

	return images, nil
}

// isHEIC tells a HEIF photo (an iPhone's HEIC) by its ftyp box brand.
func isHEIC(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}

	switch string(b[8:12]) {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
		return true
	}

	return false
}

// ListUpTo is List with a page of limit notes, clamped to 1..ListMax: the
// size the API's caller asks for.
func (s *Service) ListUpTo(ctx context.Context, actor *model.User, f Filter, limit int) (*Page, error) {
	c := *s
	c.pageSize = min(max(limit, 1), ListMax)

	return c.List(ctx, actor, f)
}
