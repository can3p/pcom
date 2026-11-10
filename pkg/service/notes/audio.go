package notes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// AudioPrefix is where in the bucket recordings are stored. Their names start
// with it, so they never pass for images.
const AudioPrefix = "audio/"

// What the actor reads when a recording is refused.
const (
	msgAudioEmpty    = "The recording is empty."
	msgAudioFormat   = "pcom keeps recordings in WebM, Ogg, MP4 (M4A) or MP3."
	msgNoRecording   = "This note has no recording."
	msgTranscribing  = "The recording is being transcribed already."
	msgSTTOff        = "Transcription is not available."
	msgPickLanguage  = "Pick a language from the list."
	msgAudioTooLarge = "The recording is larger than %d MiB."
)

// audioTypes are the containers a recording may come in, by the content type
// stored with it, and the extension its object gets.
var audioTypes = map[string]string{
	"audio/webm":  ".webm",
	"audio/ogg":   ".ogg",
	"audio/mp4":   ".m4a",
	"audio/mpeg":  ".mp3",
	"audio/x-m4a": ".m4a",
}

// languageRe is an ISO 639-1 code, what "Transcribe again as" sends.
var languageRe = regexp.MustCompile(`^[a-z]{2}$`)

// recording is an upload that passed the checks.
type recording struct {
	bytes       []byte
	contentType string
	ext         string
}

// readRecording reads a recording of at most AudioMaxBytes and recognizes its
// container from its bytes. contentType, the client's word for it, only
// refuses early what says it is something other than audio; what is stored is
// what the bytes are.
func (s *Service) readRecording(r io.Reader, contentType string) (*recording, error) {
	if contentType != "" {
		declared, _, err := mime.ParseMediaType(contentType)
		if err != nil || declared != "application/octet-stream" && audioTypes[declared] == "" {
			return nil, service.Invalid("file", msgAudioFormat)
		}
	}

	maxBytes := s.limits.AudioMaxBytes

	b, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}

	switch {
	case len(b) == 0:
		return nil, service.Invalid("file", msgAudioEmpty)
	case int64(len(b)) > maxBytes:
		return nil, service.Invalid("file", fmt.Sprintf(msgAudioTooLarge, maxBytes>>20))
	}

	ct := sniffAudio(b)
	if ct == "" {
		return nil, service.Invalid("file", msgAudioFormat)
	}

	return &recording{bytes: b, contentType: ct, ext: audioTypes[ct]}, nil
}

// sniffAudio names the container of b by its signature, or "" when it is
// none pcom keeps.
func sniffAudio(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("OggS")):
		return "audio/ogg"
	case bytes.HasPrefix(b, []byte{0x1A, 0x45, 0xDF, 0xA3}) && bytes.Contains(b[:min(len(b), 64)], []byte("webm")):
		return "audio/webm"
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		return "audio/mp4"
	case bytes.HasPrefix(b, []byte("ID3")), len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0:
		return "audio/mpeg"
	}

	return ""
}

// CreateFromAudio saves a note of the actor from a recording, as Create saves
// one from in: the note, the recording's row and, when transcription is
// on, the note pending and its notes.transcribe job, all in one transaction.
// The recording's text becomes the note's body when the job runs.
func (s *Service) CreateFromAudio(ctx context.Context, actor *model.User, r io.Reader, contentType string, in Input) (*model.Note, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	rec, err := s.readRecording(r, contentType)
	if err != nil {
		return nil, err
	}

	var note *model.Note

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := s.within(tx).Create(ctx, actor, in)
		if err != nil {
			return err
		}

		note = n

		return s.attach(ctx, tx, n, rec)
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// AttachAudio gives a note of the actor a recording, replacing the one it
// had (whose object a notes.delete_audio job removes), and transcribes it
// like CreateFromAudio. The transcript is added to the note's text.
func (s *Service) AttachAudio(ctx context.Context, actor *model.User, noteID string, r io.Reader, contentType string) (*model.Note, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	rec, err := s.readRecording(r, contentType)
	if err != nil {
		return nil, err
	}

	var note *model.Note

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := s.ownNote(ctx, tx, actor, noteID, write)
		if err != nil {
			return err
		}

		note = n

		return s.attach(ctx, tx, n, rec)
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// within is the service working on tx, so that a method that opens its own
// transaction joins tx instead.
func (s *Service) within(tx *repo.Store) *Service {
	c := *s
	c.store = tx

	return &c
}

// attach records rec as n's recording on tx, stores its object and enqueues
// its jobs on tx. A failed upload rolls the rows back before any job is
// enqueued; a failed enqueue rolls them back and removes the object again.
func (s *Service) attach(ctx context.Context, tx *repo.Store, n *model.Note, rec *recording) error {
	if s.media == nil {
		return errors.New("notes: no media storage to keep a recording in")
	}

	if s.jobs == nil {
		return errors.New("notes: no job queue for a recording's jobs")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	stored := &model.NoteRecording{
		ID:          id.String(),
		UserID:      n.UserID,
		Fname:       AudioPrefix + id.String() + rec.ext,
		ContentType: rec.contentType,
		SizeBytes:   int64(len(rec.bytes)),
	}

	if err := tx.CreateNoteRecording(ctx, stored); err != nil {
		return err
	}

	old := n.RecordingID
	n.RecordingID = &stored.ID
	n.TranscriptionError = nil
	n.TranscriptionStatus = model.NoteTranscriptionNone

	if s.stt != nil {
		n.TranscriptionStatus = model.NoteTranscriptionPending
	}

	if err := tx.SaveNote(ctx, n, "recording_id", "transcription_status", "transcription_error", "updated_at"); err != nil {
		return err
	}

	if err := s.media.UploadFile(ctx, stored.Fname, rec.bytes, rec.contentType); err != nil {
		return err
	}

	if err := s.enqueueAudioJobs(ctx, tx, n, old); err != nil {
		return errors.Join(err, s.media.DeleteFile(ctx, stored.Fname))
	}

	return nil
}

// enqueueAudioJobs enqueues the deletion of the recording n had, if any, and
// the transcription of the one it has now when transcription is on.
func (s *Service) enqueueAudioJobs(ctx context.Context, tx *repo.Store, n *model.Note, old *string) error {
	if old != nil {
		if err := tx.Enqueue(ctx, s.jobs, DeleteAudioArgs{RecordingID: *old}); err != nil {
			return err
		}
	}

	if s.stt == nil {
		return nil
	}

	return tx.Enqueue(ctx, s.jobs, TranscribeArgs{NoteID: n.ID, RecordingID: *n.RecordingID, Attempts: s.sttAttempts})
}

// Recording is a note's recording, read whole: it is at most AudioMaxBytes.
type Recording struct {
	Bytes       []byte
	ContentType string
	ModTime     time.Time
}

// Recording returns the recording of a note of the actor. Anybody else, and
// a note without one, gets not-found.
func (s *Service) Recording(ctx context.Context, actor *model.User, noteID string) (*Recording, error) {
	n, err := s.ownNote(ctx, s.store, actor, noteID, read)
	if err != nil {
		return nil, err
	}

	if n.RecordingID == nil {
		return nil, service.ErrNotFound
	}

	stored, err := s.store.NoteRecordingByID(ctx, *n.RecordingID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	b, err := s.download(ctx, stored.Fname)
	if errors.Is(err, media.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	return &Recording{Bytes: b, ContentType: stored.ContentType, ModTime: stored.CreatedAt}, nil
}

// download reads an object of the bucket whole.
func (s *Service) download(ctx context.Context, fname string) ([]byte, error) {
	if s.media == nil {
		return nil, errors.New("notes: no media storage to read a recording from")
	}

	rc, _, _, err := s.media.DownloadFile(ctx, fname)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	return io.ReadAll(rc)
}

// IsRecordingFile reports whether fname, a media file name, is a note's
// recording, which the public media route must never serve.
func IsRecordingFile(fname string) bool {
	return strings.HasPrefix(fname, AudioPrefix)
}

// Retranscribe sends the recording of a note of the actor to the backend
// again, with language forced (an ISO 639-1 code), and replaces the note's
// text with what comes back. It counts against the limits like the first
// transcription; the page asks before replacing text the actor has edited.
func (s *Service) Retranscribe(ctx context.Context, actor *model.User, noteID, language string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if s.stt == nil {
		return service.Invalid("", msgSTTOff)
	}

	language = strings.ToLower(strings.TrimSpace(language))
	if !languageRe.MatchString(language) {
		return service.Invalid("language", msgPickLanguage)
	}

	if s.jobs == nil {
		return errors.New("notes: no job queue to transcribe with")
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := s.ownNote(ctx, tx, actor, noteID, write)
		if err != nil {
			return err
		}

		switch {
		case n.RecordingID == nil:
			return service.Invalid("", msgNoRecording)
		case n.TranscriptionStatus == model.NoteTranscriptionPending:
			return service.Invalid("", msgTranscribing)
		}

		seconds, err := tx.LatestSTTSeconds(ctx, n.ID)
		if err != nil {
			return err
		}

		if err := s.within(tx).Allow(ctx, actor.ID, seconds); err != nil {
			return err
		}

		n.TranscriptionStatus = model.NoteTranscriptionPending
		n.TranscriptionError = nil

		if err := tx.SaveNote(ctx, n, "transcription_status", "transcription_error", "updated_at"); err != nil {
			return err
		}

		request, err := uuid.NewV7()
		if err != nil {
			return err
		}

		return tx.Enqueue(ctx, s.jobs, TranscribeArgs{NoteID: n.ID, RecordingID: *n.RecordingID, Language: language, Request: request.String(), Attempts: s.sttAttempts})
	})
}
