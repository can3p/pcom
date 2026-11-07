package notes_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/notes/capture"
	"github.com/can3p/pcom/pkg/service"
	mediasvc "github.com/can3p/pcom/pkg/service/media"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/stretchr/testify/require"
)

const heicHead = "\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"

func photo(data string) notes.Upload {
	return notes.Upload{Body: strings.NewReader(data), ContentType: "image/jpeg"}
}

// A capture makes one note of text, photos and a recording: the text as
// its kind stores it, the photos in order, the recording pending.
func TestCapture(t *testing.T) {
	t.Parallel()

	w, storage := audioWorld(t)
	at := w.now.Add(-time.Hour)

	n, err := w.svc.Capture(w.ctx, w.me, notes.CaptureInput{
		Text:       "  a passage\nof two lines ",
		How:        capture.Shared,
		Tags:       []string{"Walk"},
		CapturedAt: at,
		Source:     &notes.SourceInput{Kind: model.NoteSourceBook, Title: "Dune"},
		Images:     []notes.Upload{photo(pngHead + "first"), photo(pngHead + "second")},
		Audio:      &notes.Upload{Body: strings.NewReader(webmHead + "voice"), ContentType: "audio/webm"},
	})
	require.NoError(t, err)
	require.Equal(t, "> a passage\n> of two lines", n.Body)
	require.Equal(t, []string{"walk"}, n.Tags)
	require.True(t, at.Equal(n.CapturedAt))
	require.Equal(t, "Dune", n.Source.Title)
	require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
	require.Len(t, n.Images, 2)

	for i, want := range []string{"first", "second"} {
		require.Equal(t, i, n.Images[i].Position)
		rc, _, _, err := storage.DownloadFile(w.ctx, n.Images[i].Upload.UploadedFname)
		require.NoError(t, err)
		obj := testutil.Must(io.ReadAll(rc))(t)
		require.NoError(t, rc.Close())
		require.Equal(t, pngHead+want, string(obj))
	}

	require.Len(t, fakequeue.Of[notes.TranscribeArgs](w.queue), 1)

	again := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "more", Source: &notes.SourceInput{Kind: model.NoteSourceBook, Title: " dune "}}))(t)
	require.Equal(t, n.Source.ID, again.Source.ID, "an existing source is reused")
	require.True(t, w.now.Equal(again.CapturedAt), "no time is now")

	link := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "https://example.test/a", How: capture.QuoteShortcut}))(t)
	require.Equal(t, "https://example.test/a", link.Body, "a lone link stays bare")
	require.Nil(t, link.Source)
}

// A quote without a source takes the previous quote's source while that is
// recent; other notes and a quote with its own source don't.
func TestCapture_QuoteSource(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	src := testutil.Must(factory.NoteSource(w.ctx, w.db, w.me.ID))(t)
	w.note(t, w.me, factory.FromSource(src.ID), factory.CapturedAt(w.now.Add(-time.Minute)))

	quote := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "passage", How: capture.QuoteShortcut}))(t)
	require.Equal(t, "> passage", quote.Body)
	require.NotNil(t, quote.Source)
	require.Equal(t, src.ID, quote.Source.ID)

	typed := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "my own thought"}))(t)
	require.Nil(t, typed.Source)

	own := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "x", How: capture.QuoteShortcut, Source: &notes.SourceInput{Kind: model.NoteSourceBook, Title: "Other"}}))(t)
	require.Equal(t, "Other", own.Source.Title)

	w.now = w.now.Add(2 * notes.DefaultLimits.FocusIdle)
	late := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "later", How: capture.QuoteShortcut}))(t)
	require.Nil(t, late.Source, "idle for too long")
}

// The focus is applied to every capture and renewed by it.
func TestCapture_Focus(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	require.NoError(t, w.svc.SetFocus(w.ctx, w.me, "trip"))

	for range 2 {
		w.now = w.now.Add(notes.DefaultLimits.FocusIdle - time.Minute)
		n := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "seen #bird", Tags: []string{"a"}}))(t)
		require.ElementsMatch(t, []string{"a", "trip"}, n.Tags, "the focus outlives its idle time while captures renew it")
		require.Equal(t, []string{"bird"}, n.TextTags)
	}
}

// A capture is refused whole: nothing is written when the text, a photo or
// the recording is refused, or when storing a file fails.
func TestCapture_Refused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      notes.CaptureInput
		fail    bool
		wantMsg string
	}{
		{"empty", notes.CaptureInput{Text: "  "}, false, "Send some text, a photo or a recording."},
		{"not an image", notes.CaptureInput{Images: []notes.Upload{photo(pngHead + "ok"), photo("plain text")}}, false, "pcom keeps photos in JPEG, PNG or WebP."},
		{"heic", notes.CaptureInput{Images: []notes.Upload{photo(heicHead + "data")}}, false, "pcom can't read HEIC photos: add \"Convert to JPEG\" before sending the photo in the Shortcut."},
		{"not a recording", notes.CaptureInput{Text: "x", Audio: &notes.Upload{Body: strings.NewReader(pngHead)}}, false, "pcom keeps recordings in WebM, Ogg, MP4 (M4A) or MP3."},
		{"too many tags", notes.CaptureInput{Text: "x", Tags: []string{"a", "b", "c"}}, false, "A note can have at most 2 tags."},
		{"storage fails", notes.CaptureInput{Text: "x", Images: []notes.Upload{photo(pngHead + "ok")}}, true, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			w, storage := audioWorld(t, notes.WithLimits(notes.Limits{TagsMax: 2}))
			boom := errors.New("bucket down")

			if c.fail {
				storage.FailUploadWith(boom)
			}

			_, err := w.svc.Capture(w.ctx, w.me, c.in)
			if c.fail {
				require.ErrorIs(t, err, boom)
			} else {
				requireInvalid(t, err, c.wantMsg)
			}

			require.Equal(t, 0, w.count(t, "select count(*) from notes"))
			require.Equal(t, 0, w.count(t, "select count(*) from note_images"))
			require.Equal(t, 0, w.count(t, "select count(*) from media_uploads"))
		})
	}

	_, err := newWorld(t).svc.Capture(t.Context(), nil, notes.CaptureInput{Text: "x"})
	require.ErrorIs(t, err, service.ErrNeedsLogin)
}

// ListUpTo pages by the size asked, clamped to 1..ListMax.
func TestListUpTo(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	for i := range 3 {
		w.note(t, w.me, factory.CapturedAt(w.now.Add(time.Duration(i)*time.Minute)))
	}

	for limit, want := range map[int]int{-1: 1, 0: 1, 2: 2, notes.ListMax + 1: 3} {
		p := testutil.Must(w.svc.ListUpTo(w.ctx, w.me, notes.Filter{}, limit))(t)
		require.Len(t, p.Entries, want, "limit %d", limit)
	}
}

// A photo is capped at MAX_UPLOAD_SIZE, media's default when unset.
func TestCapture_PhotoSize(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		opts []notes.Option
		max  int64
	}{
		"default":    {nil, mediasvc.DefaultMaxUploadSize},
		"configured": {[]notes.Option{notes.WithMaxUploadSize(2 << 20)}, 2 << 20},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w, _ := audioWorld(t, c.opts...)
			at := pngHead + strings.Repeat("x", int(c.max)-len(pngHead))

			_, err := w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Images: []notes.Upload{photo(at + "x")}})
			requireInvalid(t, err, fmt.Sprintf("A photo can be at most %d MiB.", c.max>>20))

			n := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Images: []notes.Upload{photo(at)}}))(t)
			require.Len(t, n.Images, 1, "exactly the cap is kept")
		})
	}
}
