package notes_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/stt"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/stretchr/testify/require"
)

// Recordings as each container starts.
const (
	webmHead = "\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\x82\x84webm"
	oggHead  = "OggS\x00\x02"
	mp4Head  = "\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"
	id3Head  = "ID3\x04\x00\x00"
	mp3Frame = "\xff\xfb\x90\x00"
	pngHead  = "\x89PNG\r\n\x1a\n"
)

// unusedSTT turns transcription on in tests that never run the job.
type unusedSTT struct{}

func (unusedSTT) Name() string  { return "unused" }
func (unusedSTT) Model() string { return "unused" }
func (unusedSTT) Transcribe(context.Context, stt.Audio) (stt.Result, error) {
	return stt.Result{}, errors.New("not expected")
}

func audioWorld(t *testing.T, opts ...notes.Option) (*world, *fakestorage.Storage) {
	t.Helper()

	storage := fakestorage.New()

	return newWorld(t, append([]notes.Option{notes.WithMediaStorage(storage), notes.WithSTT(unusedSTT{})}, opts...)...), storage
}

func requireInvalid(t *testing.T, err error, msg string) {
	t.Helper()

	var invalid *service.ValidationError

	require.ErrorAs(t, err, &invalid)
	require.Equal(t, msg, invalid.Message)
}

func TestNotes_CreateFromAudio(t *testing.T) {
	t.Parallel()

	const formats = "pcom keeps recordings in WebM, Ogg, MP4 (M4A) or MP3."

	cases := []struct {
		name, header, data string
		wantType, wantMsg  string
	}{
		{"webm from the page", "audio/webm;codecs=opus", webmHead + "voice", "audio/webm", ""},
		{"ogg", "audio/ogg", oggHead + "voice", "audio/ogg", ""},
		{"m4a from a phone, as octet-stream", "application/octet-stream", mp4Head + "voice", "audio/mp4", ""},
		{"m4a named x-m4a", "audio/x-m4a", mp4Head + "voice", "audio/mp4", ""},
		{"mp3 with a tag", "audio/mpeg", id3Head + "voice", "audio/mpeg", ""},
		{"mp3 frames, no header", "", mp3Frame + "voice", "audio/mpeg", ""},
		{"a header can't make an image audio", "audio/webm", pngHead + "image", "", formats},
		{"an image header is refused", "image/png", oggHead + "voice", "", formats},
		{"empty", "audio/webm", "", "", "The recording is empty."},
		{"over the size cap", "audio/webm", webmHead + strings.Repeat("x", 2<<20), "", "The recording is larger than 1 MiB."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			w, storage := audioWorld(t, notes.WithLimits(notes.Limits{AudioMaxBytes: 1 << 20}))

			note, err := w.svc.CreateFromAudio(w.ctx, w.me, strings.NewReader(c.data), c.header, notes.Input{Tags: []string{"walk"}})
			if c.wantMsg != "" {
				requireInvalid(t, err, c.wantMsg)
				require.Equal(t, 0, w.count(t, "select count(*) from notes"))
				require.Equal(t, 0, w.count(t, "select count(*) from media_uploads"))
				require.Empty(t, w.queue.Inserted())

				return
			}

			require.NoError(t, err)
			require.Equal(t, model.NoteTranscriptionPending, note.TranscriptionStatus)
			require.Equal(t, []string{"walk"}, note.Tags)

			upload := testutil.Must(repo.New(w.db).MediaUploadByID(w.ctx, *note.AudioUploadID))(t)
			require.Equal(t, model.MediaKindNoteAudio, upload.Kind)
			require.Equal(t, c.wantType, upload.ContentType)
			require.Equal(t, w.me.ID, *upload.UserID)
			require.True(t, strings.HasPrefix(upload.UploadedFname, notes.AudioPrefix))

			stored := testutil.Must(w.svc.Recording(w.ctx, w.me, note.ID))(t)
			require.Equal(t, []byte(c.data), stored.Bytes)
			require.Equal(t, c.wantType, stored.ContentType)
			require.True(t, testutil.Must(storage.ObjectExists(w.ctx, upload.UploadedFname))(t))

			require.Equal(t, []notes.TranscribeArgs{{NoteID: note.ID, UploadID: upload.ID}}, fakequeue.Of[notes.TranscribeArgs](w.queue))
		})
	}
}

// Without a backend the recording is kept and nothing waits for a
// transcription; anybody logged out needs to log in; a failed upload to the
// bucket leaves nothing behind.
func TestNotes_CreateFromAudioEdges(t *testing.T) {
	t.Parallel()

	w := newWorld(t, notes.WithMediaStorage(fakestorage.New()))

	note, err := w.svc.CreateFromAudio(w.ctx, w.me, strings.NewReader(webmHead), "audio/webm", notes.Input{})
	require.NoError(t, err)
	require.Equal(t, model.NoteTranscriptionNone, note.TranscriptionStatus)
	require.NotNil(t, note.AudioUploadID)
	require.Empty(t, w.queue.Inserted())

	_, err = w.svc.CreateFromAudio(w.ctx, nil, strings.NewReader(webmHead), "audio/webm", notes.Input{})
	require.ErrorIs(t, err, service.ErrNeedsLogin)

	broken, storage := audioWorld(t)
	storage.FailUploadWith(errors.New("bucket down"))

	_, err = broken.svc.CreateFromAudio(broken.ctx, broken.me, strings.NewReader(webmHead), "audio/webm", notes.Input{})
	require.Error(t, err)
	require.Equal(t, 0, broken.count(t, "select count(*) from notes"))
	require.Equal(t, 0, broken.count(t, "select count(*) from media_uploads"))
}

// A new recording replaces the note's old one, which a job deletes.
func TestNotes_AttachAudio(t *testing.T) {
	t.Parallel()

	w, _ := audioWorld(t)
	old := testutil.Must(factory.NoteAudio(w.ctx, w.db, w.me.ID))(t)
	note := w.note(t, w.me, factory.WithRecording(old.ID), factory.WithNoteBody("a photo of a page"))

	got, err := w.svc.AttachAudio(w.ctx, w.me, note.ID, strings.NewReader(oggHead), "audio/ogg")
	require.NoError(t, err)
	require.NotEqual(t, old.ID, *got.AudioUploadID)
	require.Equal(t, model.NoteTranscriptionPending, got.TranscriptionStatus)
	require.Equal(t, "a photo of a page", got.Body)
	require.Equal(t, []notes.DeleteAudioArgs{{UploadID: old.ID}}, fakequeue.Of[notes.DeleteAudioArgs](w.queue))
	require.Equal(t, []notes.TranscribeArgs{{NoteID: note.ID, UploadID: *got.AudioUploadID}}, fakequeue.Of[notes.TranscribeArgs](w.queue))

	_, err = w.svc.AttachAudio(w.ctx, w.other, note.ID, strings.NewReader(oggHead), "audio/ogg")
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestNotes_Recording(t *testing.T) {
	t.Parallel()

	w, _ := audioWorld(t)
	note := testutil.Must(w.svc.CreateFromAudio(w.ctx, w.me, strings.NewReader(mp4Head+"memo"), "audio/mp4", notes.Input{}))(t)
	plain := w.note(t, w.me)

	rec, err := w.svc.Recording(w.ctx, w.me, note.ID)
	require.NoError(t, err)
	require.Equal(t, []byte(mp4Head+"memo"), rec.Bytes)

	for _, c := range []struct {
		name  string
		actor *model.User
		id    string
		want  error
	}{
		{"somebody else", w.other, note.ID, service.ErrNotFound},
		{"anonymous", nil, note.ID, service.ErrNeedsLogin},
		{"a note without a recording", w.me, plain.ID, service.ErrNotFound},
	} {
		_, err := w.svc.Recording(w.ctx, c.actor, c.id)
		require.ErrorIs(t, err, c.want, c.name)
	}
}

func TestNotes_Retranscribe(t *testing.T) {
	t.Parallel()

	w, _ := audioWorld(t, notes.WithLimits(notes.Limits{STTMinutesPerDay: 1}))
	audio := testutil.Must(factory.NoteAudio(w.ctx, w.db, w.me.ID))(t)
	note := w.note(t, w.me, factory.WithRecording(audio.ID))
	plain := w.note(t, w.me)
	testutil.Must(factory.STTRequest(w.ctx, w.db, w.me.ID, note.ID, 30))(t)

	require.NoError(t, w.svc.Retranscribe(w.ctx, w.me, note.ID, " RU "))
	queued := fakequeue.Of[notes.TranscribeArgs](w.queue)
	require.Len(t, queued, 1)
	require.NotEmpty(t, queued[0].Request)
	require.Equal(t, notes.TranscribeArgs{NoteID: note.ID, UploadID: audio.ID, Language: "ru", Request: queued[0].Request}, queued[0])
	require.Equal(t, model.NoteTranscriptionPending, testutil.Must(w.svc.Get(w.ctx, w.me, note.ID))(t).TranscriptionStatus)

	requireInvalid(t, w.svc.Retranscribe(w.ctx, w.me, note.ID, "en"), "The recording is being transcribed already.")
	requireInvalid(t, w.svc.Retranscribe(w.ctx, w.me, plain.ID, "en"), "This note has no recording.")
	requireInvalid(t, w.svc.Retranscribe(w.ctx, w.me, note.ID, "russian"), "Pick a language from the list.")
	require.ErrorIs(t, w.svc.Retranscribe(w.ctx, w.other, note.ID, "en"), service.ErrNotFound)

	// 30 seconds used and 30 more fit the minute; with 55 used, the 25 seconds
	// of the recording don't.
	again := w.note(t, w.me, factory.WithRecording(testutil.Must(factory.NoteAudio(w.ctx, w.db, w.me.ID))(t).ID))
	testutil.Must(factory.STTRequest(w.ctx, w.db, w.me.ID, again.ID, 25))(t)
	err := w.svc.Retranscribe(w.ctx, w.me, again.ID, "en")

	var invalid *service.ValidationError

	require.ErrorAs(t, err, &invalid)
	require.Contains(t, invalid.Message, "of your 1 minutes of transcription")
	require.Len(t, w.queue.Inserted(), 1)

	off := newWorld(t)
	requireInvalid(t, off.svc.Retranscribe(off.ctx, off.me, note.ID, "en"), "Transcription is not available.")
}

// The public media route refuses recordings by name.
func TestNotes_IsRecordingFile(t *testing.T) {
	t.Parallel()

	w, _ := audioWorld(t)
	note := testutil.Must(w.svc.CreateFromAudio(w.ctx, w.me, bytes.NewReader([]byte(webmHead)), "", notes.Input{}))(t)
	upload := testutil.Must(repo.New(w.db).MediaUploadByID(w.ctx, *note.AudioUploadID))(t)

	require.True(t, notes.IsRecordingFile(upload.UploadedFname))
	require.False(t, notes.IsRecordingFile("3fa85f64-5717-4562-b3fc-2c963f66afa6.png"))
}
