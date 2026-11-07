package notes

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/stt"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// TranscribeArgs is the job turning a note's recording into its text. It is
// unique per note and recording while it waits or runs, so a double tap
// transcribes once; each Retranscribe is a request of its own, since the job
// before it may still be finishing when the note shows done.
type TranscribeArgs struct {
	NoteID   string `json:"note_id" river:"unique"`
	UploadID string `json:"upload_id" river:"unique"`
	// Language forces the language (Retranscribe); empty lets the backend
	// detect it. A forced transcription replaces the text instead of adding
	// to it.
	Language string `json:"language,omitempty"`
	// Request tells a Retranscribe apart from the jobs before it.
	Request string `json:"request,omitempty" river:"unique"`
	// Attempts is how many times the job runs before the note is marked
	// failed (STT_ATTEMPTS); zero is DefaultSTTAttempts.
	Attempts int `json:"attempts,omitempty"`
}

// Kind names the job kind.
func (TranscribeArgs) Kind() string { return "notes.transcribe" }

// InsertOpts puts the job on the stt queue with its attempts, unique per note
// and recording until it finishes. River reads it from the args inserted.
func (a TranscribeArgs) InsertOpts() river.InsertOpts {
	attempts := a.Attempts
	if attempts <= 0 {
		attempts = DefaultSTTAttempts
	}

	return river.InsertOpts{
		Queue:       QueueSTT,
		MaxAttempts: attempts,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
				rivertype.JobStateRetryable, rivertype.JobStateScheduled,
			},
		},
	}
}

// Plugins tells the worker, through its context, whether it runs the job's
// last attempt.
func (TranscribeArgs) Plugins() []rivertype.Plugin {
	return []rivertype.Plugin{river.WorkerMiddlewareFunc(
		func(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
			return doInner(context.WithValue(ctx, lastAttemptKey{}, job.Attempt >= job.MaxAttempts))
		})}
}

type lastAttemptKey struct{}

// lastAttempt reports whether the job being worked won't be retried.
func lastAttempt(ctx context.Context) bool {
	last, _ := ctx.Value(lastAttemptKey{}).(bool)

	return last
}

// Transcribe is the notes.transcribe worker. It re-checks that the note
// still waits for this recording (it may be deleted, or its recording
// replaced, since), sends the recording to the backend and writes the text,
// done and the stt_requests row in one transaction. A failed call is
// retried; the last attempt, or a refusal by the limits, marks the note
// failed with the reason.
func (s *Service) Transcribe(ctx context.Context, args TranscribeArgs) error {
	err := s.transcribe(ctx, args)

	var invalid *service.ValidationError

	switch {
	case err == nil:
		return nil
	case errors.As(err, &invalid):
		return river.JobCancel(s.markFailed(ctx, args, invalid.Message, err))
	case lastAttempt(ctx):
		return s.markFailed(ctx, args, err.Error(), err)
	}

	return err
}

// current reports whether n still waits for the transcription of args.
func current(n *model.Note, args TranscribeArgs) bool {
	return n.AudioUploadID != nil && *n.AudioUploadID == args.UploadID && n.TranscriptionStatus == model.NoteTranscriptionPending
}

func (s *Service) transcribe(ctx context.Context, args TranscribeArgs) error {
	if s.stt == nil {
		return errors.New("notes: transcription is off")
	}

	note, err := s.store.NoteByID(ctx, args.NoteID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	if !current(note, args) {
		return nil
	}

	upload, err := s.store.MediaUploadByID(ctx, args.UploadID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	if err := s.Allow(ctx, note.UserID, 0); err != nil {
		return err
	}

	audio, err := s.download(ctx, upload.UploadedFname)
	if err != nil {
		return err
	}

	res, callErr := s.stt.Transcribe(ctx, stt.Audio{Bytes: audio, ContentType: upload.ContentType, Language: args.Language})

	call := &model.STTRequest{
		UserID: note.UserID, Provider: s.stt.Name(), Model: s.stt.Model(),
		Seconds: int(math.Ceil(res.Seconds)), Status: model.STTStatusOK,
	}
	if callErr != nil {
		call.Status, call.Seconds = model.STTStatusFailed, 0
	}

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := tx.LockNote(ctx, args.NoteID)
		if errors.Is(err, repo.ErrNotFound) {
			// Deleted while the backend listened: the call still counts.
			return tx.InsertSTTRequest(ctx, call)
		} else if err != nil {
			return err
		}

		call.NoteID = &n.ID
		if err := tx.InsertSTTRequest(ctx, call); err != nil {
			return err
		}

		if callErr != nil || !current(n, args) {
			return nil
		}

		return s.writeTranscript(ctx, tx, n, res.Text, args.Language != "")
	})

	return errors.Join(callErr, err)
}

// writeTranscript makes text, its spoken hashtags written as #tags, the
// note's body (replace) or adds it to the body, and marks the note done. A
// body the limits refuse marks it failed instead.
func (s *Service) writeTranscript(ctx context.Context, tx *repo.Store, n *model.Note, text string, replace bool) error {
	text = spokenHashtags(strings.TrimSpace(text))

	body := text
	if !replace && strings.TrimSpace(n.Body) != "" {
		body = strings.TrimRight(n.Body, "\n") + "\n\n" + text
	}

	set, textTags, err := s.tagsOf(body, n.Tags)

	var invalid *service.ValidationError

	switch {
	case errors.As(err, &invalid):
		n.TranscriptionStatus, n.TranscriptionError = model.NoteTranscriptionFailed, &invalid.Message
	case err != nil:
		return err
	default:
		n.Body, n.Tags, n.TextTags = body, set, textTags
		n.TranscriptionStatus, n.TranscriptionError = model.NoteTranscriptionDone, nil
	}

	return tx.SaveNote(ctx, n, "body", "tags", "text_tags", "transcription_status", "transcription_error", "updated_at")
}

// markFailed marks the note failed with msg if it still waits for the
// transcription of args, and returns cause.
func (s *Service) markFailed(ctx context.Context, args TranscribeArgs, msg string, cause error) error {
	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := tx.LockNote(ctx, args.NoteID)
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}

		if !current(n, args) {
			return nil
		}

		n.TranscriptionStatus, n.TranscriptionError = model.NoteTranscriptionFailed, &msg

		return tx.SaveNote(ctx, n, "transcription_status", "transcription_error", "updated_at")
	})

	return errors.Join(cause, err)
}

// spokenHashtagRe finds a tag said aloud, as Whisper writes it: "hashtag
// word", "Hashtag, word.", "hash tag word", "hash-tag word", and the Russian
// "хэштег слово". Group 1 is what comes before, group 2 the word.
var spokenHashtagRe = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_#])(?:hash[\s-]?tag|х[эе]штег)[\s,.:;-]+(\p{L}[\p{L}\p{N}_]*(?:-[\p{L}\p{N}_]+)*)`)

// spokenHashtags writes the tags said in text as #tags, lowercased, so they
// become the note's tags like typed ones.
func spokenHashtags(text string) string {
	return spokenHashtagRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := spokenHashtagRe.FindStringSubmatch(m)

		return sub[1] + "#" + strings.ToLower(sub[2])
	})
}

// DeleteAudio is the notes.delete_audio worker: it removes a recording no
// note uses any more from the bucket, and its upload row. An upload that is
// gone, is not a recording or is in use again is left alone.
func (s *Service) DeleteAudio(ctx context.Context, args DeleteAudioArgs) error {
	if s.media == nil {
		return errors.New("notes: no media storage to delete a recording from")
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		upload, err := tx.MediaUploadByID(ctx, args.UploadID)
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}

		if upload.Kind != model.MediaKindNoteAudio {
			return nil
		}

		if used, err := tx.NoteUsesAudio(ctx, upload.ID); err != nil || used {
			return err
		}

		if err := tx.DeleteMediaUpload(ctx, upload.ID); err != nil {
			return err
		}

		return s.media.DeleteFile(ctx, upload.UploadedFname)
	})
}
