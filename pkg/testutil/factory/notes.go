package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
)

// NoteOpt customizes a Note before it is inserted.
type NoteOpt func(*model.Note)

// WithNoteBody overrides the made-up body.
func WithNoteBody(body string) NoteOpt {
	return func(n *model.Note) { n.Body = body }
}

// WithNoteTags sets the tags set by actions.
func WithNoteTags(tags ...string) NoteOpt {
	return func(n *model.Note) { n.Tags = tags }
}

// WithNoteTextTags sets the tags derived from the body; the factory doesn't
// parse the body.
func WithNoteTextTags(tags ...string) NoteOpt {
	return func(n *model.Note) { n.TextTags = tags }
}

// CapturedAt sets when the note was captured.
func CapturedAt(t time.Time) NoteOpt {
	return func(n *model.Note) { n.CapturedAt = t }
}

// FromSource makes the note come from a source.
func FromSource(sourceID string) NoteOpt {
	return func(n *model.Note) { n.SourceID = &sourceID }
}

// WithRecording gives the note a recording, the upload of a note_audio.
func WithRecording(uploadID string) NoteOpt {
	return func(n *model.Note) {
		n.AudioUploadID = &uploadID
		n.TranscriptionStatus = model.NoteTranscriptionDone
	}
}

// Note inserts a note of userID with a made-up body and no tags, captured
// now.
func Note(ctx context.Context, exec repo.Executor, userID string, opts ...NoteOpt) (*model.Note, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := &model.Note{
		ID:         id,
		UserID:     userID,
		Body:       fmt.Sprintf("note body %d", next()),
		Tags:       []string{},
		TextTags:   []string{},
		CapturedAt: time.Now().UTC(),
	}

	for _, o := range opts {
		o(n)
	}

	return insertRow(ctx, exec, n)
}

// NoteSource inserts a book of userID with a made-up title.
func NoteSource(ctx context.Context, exec repo.Executor, userID string) (*model.NoteSource, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Book %d", next())

	return insertRow(ctx, exec, &model.NoteSource{
		ID: id, UserID: userID, Kind: model.NoteSourceBook, Title: title, MatchKey: title + "\x1f",
	})
}

// NoteAudio records a recording of userID, an upload of kind note_audio.
func NoteAudio(ctx context.Context, exec repo.Executor, userID string) (*model.MediaUpload, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	return insertRow(ctx, exec, &model.MediaUpload{
		ID:            id,
		UserID:        &userID,
		UploadedFname: fmt.Sprintf("audio/test-%d.webm", next()),
		ContentType:   "audio/webm",
		Kind:          model.MediaKindNoteAudio,
	})
}

// NoteImage attaches an upload to a note as its photo at position.
func NoteImage(ctx context.Context, exec repo.Executor, noteID, uploadID string, position int) (*model.NoteImage, error) {
	return insertRow(ctx, exec, &model.NoteImage{NoteID: noteID, UploadID: uploadID, Position: position})
}

// NotePostLink records the note as used in the post.
func NotePostLink(ctx context.Context, exec repo.Executor, noteID, postID string) (*model.NotePostLink, error) {
	return insertRow(ctx, exec, &model.NotePostLink{NoteID: noteID, PostID: postID, How: model.NotePostLinkInserted})
}

// STTRequest records one ok call to a speech-to-text backend for the note.
func STTRequest(ctx context.Context, exec repo.Executor, userID, noteID string, seconds int) (*model.STTRequest, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	return insertRow(ctx, exec, &model.STTRequest{
		ID: id, UserID: userID, NoteID: &noteID, Provider: "test", Model: "test", Seconds: seconds, Status: model.STTStatusOK,
	})
}
