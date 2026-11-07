package notes_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/stt"
	"github.com/can3p/pcom/pkg/stt/openaicompat"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	wm "github.com/wiremock/go-wiremock"
)

func TestMain(m *testing.M) {
	wiremock.Register("testdata/wiremock")

	code := m.Run()
	_ = wiremock.Cleanup()

	os.Exit(code)
}

// jobWorld is a world whose service enqueues on River and transcribes through
// the OpenAI-compatible backend against WireMock, one call per attempt.
type jobWorld struct {
	*world
	storage *fakestorage.Storage
	wm      *wiremock.WireMock
}

func newJobWorld(t *testing.T, opts ...notes.Option) *jobWorld {
	t.Helper()

	w := &jobWorld{world: newWorld(t), storage: fakestorage.New(), wm: wiremock.Shared(t)}
	backend := stt.Retry(openaicompat.New(openaicompat.Options{Endpoint: w.wm.Watch(t, "stt"), Model: "whisper-test", Key: "test-key"}),
		stt.RetryOptions{Attempts: 1})
	inserter := testutil.Must(jobs.NewInserter(w.db))(t)
	w.svc = notes.New(repo.New(w.db), append([]notes.Option{notes.WithJobs(inserter), notes.WithMediaStorage(w.storage), notes.WithSTT(backend)}, opts...)...)

	return w
}

// work starts a runner over the service's workers, stopped at cleanup.
func (w *jobWorld) work(t *testing.T) {
	t.Helper()

	reg := jobs.NewRegistry(repo.New(w.db), fakesender.New().Delivery())
	w.svc.RegisterJobs(reg)

	runner := testutil.Must(jobs.NewRunner(w.db, reg, jobs.WithPollInterval(10*time.Millisecond), jobs.WithQueue(notes.QueueSTT, 2)))(t)
	require.NoError(t, runner.Start(w.ctx))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, runner.Stop(ctx))
	})
}

func (w *jobWorld) record(t *testing.T, marker string) *model.Note {
	t.Helper()

	return testutil.Must(w.svc.CreateFromAudio(w.ctx, w.me, strings.NewReader(webmHead+marker), "audio/webm", notes.Input{Body: "On the way home", Tags: []string{"walk"}}))(t)
}

func (w *jobWorld) waitStatus(t *testing.T, noteID string, want model.NoteTranscriptionStatus) *model.Note {
	t.Helper()

	var note *model.Note

	require.Eventually(t, func() bool {
		n, err := w.svc.Get(w.ctx, w.me, noteID)
		note = n

		return err == nil && n.TranscriptionStatus == want
	}, 20*time.Second, 50*time.Millisecond, "the note should become %s", want)

	return note
}

func sentSTT(marker string) *wm.Request {
	return wm.NewRequest("POST", wm.URLPathEqualTo("/stt/audio/transcriptions")).WithBodyPattern(wm.Contains(marker))
}

// The transcript is added to the body with its spoken tags as #tags, and the
// call is recorded; transcribing again in a chosen language replaces the body.
func TestNotes_TranscribeJob(t *testing.T) {
	t.Parallel()

	w := newJobWorld(t)
	marker := "wm-notes-hashtags-" + uuid.NewString()
	note := w.record(t, marker)
	w.work(t)

	got := w.waitStatus(t, note.ID, model.NoteTranscriptionDone)
	require.Equal(t, "On the way home\n\nBuy oat milk. #groceries, #errands.", got.Body)
	require.Equal(t, []string{"walk"}, got.Tags)
	require.Equal(t, []string{"groceries", "errands"}, got.TextTags)
	require.Nil(t, got.TranscriptionError)
	require.Equal(t, 1, w.count(t, "select count(*) from stt_requests where note_id = $1 and status = 'ok' and seconds = 7 and provider = 'openai-compat'", note.ID))

	require.NoError(t, w.svc.Retranscribe(w.ctx, w.me, note.ID, "ru"))

	got = w.waitStatus(t, note.ID, model.NoteTranscriptionDone)
	require.Equal(t, "Купить овсяное молоко. #покупки.", got.Body)
	require.Equal(t, []string{"покупки"}, got.TextTags)
	require.Equal(t, 2, w.count(t, "select count(*) from stt_requests where note_id = $1 and status = 'ok'", note.ID))
	w.wm.Verify(t, 2, sentSTT(marker))
}

// A note deleted before its job runs is not transcribed, and its recording
// leaves the bucket.
func TestNotes_TranscribeJobAfterDelete(t *testing.T) {
	t.Parallel()

	w := newJobWorld(t)
	note := w.record(t, "wm-notes-deleted")
	fname := testutil.Must(repo.New(w.db).MediaUploadByID(w.ctx, *note.AudioUploadID))(t).UploadedFname

	require.NoError(t, w.svc.Delete(w.ctx, w.me, note.ID))
	w.work(t)

	require.Eventually(t, func() bool {
		return w.count(t, "select count(*) from river_job where state = 'completed'") == 2
	}, 20*time.Second, 50*time.Millisecond, "the transcription and the deletion should both finish")

	require.False(t, testutil.Must(w.storage.ObjectExists(w.ctx, fname))(t))
	_, err := repo.New(w.db).MediaUploadByID(w.ctx, *note.AudioUploadID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	require.Equal(t, 0, w.count(t, "select count(*) from stt_requests"))
	w.wm.Verify(t, 0, sentSTT("wm-notes-deleted"))
}

// A recording replaced before its job runs is not transcribed, and the note
// keeps what the replacement gave it; the old recording leaves the bucket.
func TestNotes_TranscribeJobAfterReplace(t *testing.T) {
	t.Parallel()

	w := newJobWorld(t)
	note := w.record(t, "wm-notes-replaced")
	old := testutil.Must(repo.New(w.db).MediaUploadByID(w.ctx, *note.AudioUploadID))(t)

	// replaced by a service with transcription off, so only the old job runs
	inserter := testutil.Must(jobs.NewInserter(w.db))(t)
	off := notes.New(repo.New(w.db), notes.WithJobs(inserter), notes.WithMediaStorage(w.storage))
	replaced := testutil.Must(off.AttachAudio(w.ctx, w.me, note.ID, strings.NewReader(oggHead), "audio/ogg"))(t)
	w.work(t)

	require.Eventually(t, func() bool {
		return w.count(t, "select count(*) from river_job where state = 'completed'") == 2
	}, 20*time.Second, 50*time.Millisecond, "the old transcription and the deletion should both finish")

	got := testutil.Must(w.svc.Get(w.ctx, w.me, note.ID))(t)
	require.Equal(t, note.Body, got.Body)
	require.Equal(t, model.NoteTranscriptionNone, got.TranscriptionStatus)
	require.Equal(t, *replaced.AudioUploadID, *got.AudioUploadID)
	require.Equal(t, 0, w.count(t, "select count(*) from stt_requests"))
	require.False(t, testutil.Must(w.storage.ObjectExists(w.ctx, old.UploadedFname))(t))
	w.wm.Verify(t, 0, sentSTT("wm-notes-replaced"))
}

// Deleting a recording whose object is gone already removes its row.
func TestNotes_DeleteAudioWithoutObject(t *testing.T) {
	t.Parallel()

	w, _ := audioWorld(t)
	upload := testutil.Must(factory.NoteAudio(w.ctx, w.db, w.me.ID))(t)

	require.NoError(t, w.svc.DeleteAudio(w.ctx, notes.DeleteAudioArgs{UploadID: upload.ID}))
	_, err := repo.New(w.db).MediaUploadByID(w.ctx, upload.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}

// A failed call is retried with the note still pending; the last attempt
// marks it failed with the error, and every call is recorded.
func TestNotes_TranscribeJobLastAttemptFails(t *testing.T) {
	t.Parallel()

	w := newJobWorld(t, notes.WithSTTAttempts(2))
	note := w.record(t, "wm-notes-unavailable")
	w.work(t)

	require.Eventually(t, func() bool {
		return w.count(t, "select count(*) from river_job where attempt = 1 and errors is not null") == 1
	}, 20*time.Second, 20*time.Millisecond, "the first attempt should fail and wait for a retry")
	require.Equal(t, model.NoteTranscriptionPending, testutil.Must(w.svc.Get(w.ctx, w.me, note.ID))(t).TranscriptionStatus)

	got := w.waitStatus(t, note.ID, model.NoteTranscriptionFailed)
	require.Contains(t, *got.TranscriptionError, "503")
	require.Equal(t, 2, w.count(t, "select count(*) from stt_requests where note_id = $1 and status = 'failed'", note.ID))
	require.Eventually(t, func() bool {
		return w.count(t, "select count(*) from river_job where state = 'discarded'") == 1
	}, 20*time.Second, 20*time.Millisecond, "the job should not be retried after its last attempt")
}

// One transcription waits per note and recording, whoever enqueues it twice;
// another recording of the note is a job of its own.
func TestNotes_TranscribeJobIsUnique(t *testing.T) {
	t.Parallel()

	w := newJobWorld(t)
	note := w.record(t, "wm-notes-unique")
	inserter := testutil.Must(jobs.NewInserter(w.db))(t)

	require.NoError(t, inserter.Insert(w.ctx, w.db, notes.TranscribeArgs{NoteID: note.ID, UploadID: *note.AudioUploadID}))
	require.Equal(t, 1, w.count(t, "select count(*) from river_job where kind = 'notes.transcribe' and queue = $1", notes.QueueSTT))

	require.NoError(t, inserter.Insert(w.ctx, w.db, notes.TranscribeArgs{NoteID: note.ID, UploadID: uuid.NewString()}))
	require.Equal(t, 2, w.count(t, "select count(*) from river_job where kind = 'notes.transcribe'"))
}
