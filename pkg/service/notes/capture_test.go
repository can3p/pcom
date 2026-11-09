package notes_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/media"
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
		Source:     "Dune",
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

	again := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "more", Source: " dune "}))(t)
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

	own := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "x", How: capture.QuoteShortcut, Source: "Other"}))(t)
	require.Equal(t, "Other", own.Source.Title)

	w.now = w.now.Add(2 * notes.DefaultQuoteSourceWindow)
	late := testutil.Must(w.svc.Capture(w.ctx, w.me, notes.CaptureInput{Text: "later", How: capture.QuoteShortcut}))(t)
	require.Nil(t, late.Source, "idle for too long")
}

// A capture is refused whole: nothing is written, enqueued or left in the
// storage when the text, a photo or the recording is refused, when storing a
// file fails, or when the recording fails after the photos are stored.
func TestCapture_Refused(t *testing.T) {
	t.Parallel()

	boom := errors.New("bucket down")
	long := strings.Repeat("x", notes.DefaultSourceTitleMaxLength+1)
	audio := func(data string) *notes.Upload { return &notes.Upload{Body: strings.NewReader(data)} }

	cases := []struct {
		name      string
		in        notes.CaptureInput
		opts      []notes.Option
		failStore bool
		wantErr   string
		field     string
		wantMsg   string
	}{
		{name: "empty", in: notes.CaptureInput{Text: "  "}, field: "text", wantMsg: "Send some text, a photo or a recording."},
		{name: "not an image", in: notes.CaptureInput{Images: []notes.Upload{photo(pngHead + "ok"), photo("plain text")}}, field: "image", wantMsg: "pcom keeps photos in JPEG, PNG or WebP."},
		{name: "heic that is not a photo", in: notes.CaptureInput{Images: []notes.Upload{photo(heicHead + "data")}}, field: "image", wantMsg: "pcom keeps photos in JPEG, PNG or WebP."},
		{name: "not a recording", in: notes.CaptureInput{Text: "x", Audio: audio(pngHead)}, field: "audio", wantMsg: "pcom keeps recordings in WebM, Ogg, MP4 (M4A) or MP3."},
		{name: "recording too large", in: notes.CaptureInput{Text: "x", Audio: audio(webmHead + strings.Repeat("x", 1<<20))},
			opts: []notes.Option{notes.WithLimits(notes.Limits{AudioMaxBytes: 1 << 20})}, field: "audio", wantMsg: "The recording is larger than 1 MiB."},
		{name: "too many tags", in: notes.CaptureInput{Text: "x", Tags: []string{"a", "b", "c"}}, opts: []notes.Option{notes.WithLimits(notes.Limits{TagsMax: 2})},
			field: "tags", wantMsg: "A note can have at most 2 tags."},
		{name: "source too long", in: notes.CaptureInput{Text: "x", Source: long}, field: "source",
			wantMsg: fmt.Sprintf("A title can be at most %d characters long.", notes.DefaultSourceTitleMaxLength)},
		{name: "storage fails", in: notes.CaptureInput{Text: "x", Images: []notes.Upload{photo(pngHead + "ok")}}, failStore: true, wantErr: boom.Error()},
		{name: "the recording fails after the photos are stored",
			in:      notes.CaptureInput{Images: []notes.Upload{photo(pngHead + "a"), photo(pngHead + "b")}, Audio: audio(webmHead + "voice")},
			opts:    []notes.Option{notes.WithJobs(nil)},
			wantErr: "notes: no job queue for a recording's jobs"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			w, storage := audioWorld(t, c.opts...)

			if c.failStore {
				storage.FailUploadWith(boom)
			}

			_, err := w.svc.Capture(w.ctx, w.me, c.in)
			if c.wantErr != "" {
				require.ErrorContains(t, err, c.wantErr)
			} else {
				var invalid *service.ValidationError
				require.ErrorAs(t, err, &invalid)
				require.Equal(t, c.field, invalid.Field)
				require.Equal(t, c.wantMsg, invalid.Message)
			}

			require.Equal(t, 0, w.count(t, "select count(*) from notes"))
			require.Equal(t, 0, w.count(t, "select count(*) from note_images"))
			require.Equal(t, 0, w.count(t, "select count(*) from media_uploads"))
			require.Empty(t, w.queue.Inserted())
			require.Empty(t, storage.Names(), "no file is left in the storage")
		})
	}

	_, err := newWorld(t).svc.Capture(t.Context(), nil, notes.CaptureInput{Text: "x"})
	require.ErrorIs(t, err, service.ErrNeedsLogin)
}

// ListUpTo pages by the size asked, clamped to 1..ListMax.
func TestListUpTo(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	for i := range notes.ListMax + 1 {
		w.note(t, w.me, factory.CapturedAt(w.now.Add(time.Duration(i)*time.Second)))
	}

	for limit, want := range map[int]int{-1: 1, 0: 1, 2: 2, notes.ListMax: notes.ListMax, notes.ListMax + 1: notes.ListMax, 1000: notes.ListMax} {
		p := testutil.Must(w.svc.ListUpTo(w.ctx, w.me, notes.Filter{}, limit))(t)
		require.Len(t, p.Entries, want, "limit %d", limit)
		require.NotEmpty(t, p.Next, "limit %d: more notes follow", limit)
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

// A HEIC photo is stored as the JPEG it is converted to.
func TestCapture_HEICIsStoredAsJPEG(t *testing.T) {
	t.Parallel()

	heic, err := os.ReadFile("testdata/photo.heic")
	require.NoError(t, err)

	if _, err := media.HEICToJPEG(heic); errors.Is(err, media.ErrHEICUnsupported) {
		if os.Getenv("CI") != "" {
			t.Fatal("libvips can't decode HEIC: CI must decode HEIC (libheif with an HEVC decoder)")
		}

		t.Skip("libvips can't decode HEIC")
	} else {
		// the converter's own error says more than the capture's field message
		require.NoError(t, err)
	}

	w, storage := audioWorld(t)

	n, err := w.svc.Capture(w.ctx, w.me, notes.CaptureInput{
		Images: []notes.Upload{{Body: bytes.NewReader(heic), ContentType: "image/heic"}},
	})
	require.NoError(t, err)
	require.Len(t, n.Images, 1)

	rc, _, _, err := storage.DownloadFile(w.ctx, n.Images[0].Upload.UploadedFname)
	require.NoError(t, err)

	stored := testutil.Must(io.ReadAll(rc))(t)
	require.NoError(t, rc.Close())

	_, ctype, err := media.DetectAndValidateImageTypeAndExt(stored)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ctype)
	require.True(t, strings.HasSuffix(n.Images[0].Upload.UploadedFname, ".jpg"), n.Images[0].Upload.UploadedFname)
}
