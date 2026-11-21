package notes

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// DeleteAudioArgs is the job deleting a note's recording, its object and its
// row, enqueued in the transaction that deletes the note or replaces the
// recording.
type DeleteAudioArgs struct {
	RecordingID string `json:"recording_id"`
}

// Kind names the job kind.
func (DeleteAudioArgs) Kind() string { return "notes.delete_audio" }

// Input is a new note.
type Input struct {
	Body string
	// Tags are set by an action; the #words of Body are tags anyway.
	Tags []string
	// SourceID is one of the actor's sources, or "" for none.
	SourceID string
	// CapturedAt is when the note was taken; the zero value means now.
	CapturedAt time.Time
}

// Create saves a note of the actor.
func (s *Service) Create(ctx context.Context, actor *model.User, in Input) (*model.Note, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	var note *model.Note

	err = s.store.Tx(ctx, func(tx *repo.Store) error {
		set, text, err := s.tagsOf(in.Body, in.Tags)
		if err != nil {
			return err
		}

		note = &model.Note{ID: id.String(), UserID: actor.ID, Body: in.Body, Tags: set, TextTags: text, CapturedAt: in.CapturedAt}
		if note.CapturedAt.IsZero() {
			note.CapturedAt = s.now()
		}

		if err := s.setSource(ctx, tx, actor, note, in.SourceID); err != nil {
			return err
		}

		return tx.CreateNote(ctx, note)
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// Edit is what a note becomes. Tags replace the ones set by hand; the ones
// from the text follow Body.
type Edit struct {
	Body string
	Tags []string
	// SourceID is one of the actor's sources, or "" for none.
	SourceID string
}

// Update changes a note of the actor.
func (s *Service) Update(ctx context.Context, actor *model.User, noteID string, in Edit) (*model.Note, error) {
	var note *model.Note

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := s.ownNote(ctx, tx, actor, noteID, write)
		if err != nil {
			return err
		}

		n.Body = in.Body
		if n.Tags, n.TextTags, err = s.tagsOf(in.Body, in.Tags); err != nil {
			return err
		}

		if err := s.setSource(ctx, tx, actor, n, in.SourceID); err != nil {
			return err
		}

		note = n

		return tx.SaveNote(ctx, n, "body", "tags", "text_tags", "source_id", "updated_at")
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// setSource sets the note's source to the actor's source id, or to none for "".
func (s *Service) setSource(ctx context.Context, tx *repo.Store, actor *model.User, n *model.Note, id string) error {
	n.SourceID, n.Source = nil, nil

	if id == "" {
		return nil
	}

	src, err := s.ownSource(ctx, tx, actor, id)
	if err != nil {
		return asField(err, "source")
	}

	n.SourceID, n.Source = &src.ID, src

	return nil
}

// Get returns a note of the actor with its source and photos.
func (s *Service) Get(ctx context.Context, actor *model.User, noteID string) (*model.Note, error) {
	if _, err := s.ownNote(ctx, s.store, actor, noteID, read); err != nil {
		return nil, err
	}

	return s.store.NoteWithRelations(ctx, noteID)
}

// PostLinks is in how many posts the actor's note was inserted, which its
// delete confirmation says.
func (s *Service) PostLinks(ctx context.Context, actor *model.User, noteID string) (int, error) {
	if _, err := s.ownNote(ctx, s.store, actor, noteID, read); err != nil {
		return 0, err
	}

	return s.store.NotePostLinkCount(ctx, noteID)
}

// Delete removes a note of the actor for good: its photos and post links
// with it, its recording from the bucket by a job. The transcription requests
// stay, so the daily limits still count them. A note that is gone already is
// deleted.
func (s *Service) Delete(ctx context.Context, actor *model.User, noteID string) error {
	_, err := s.DeleteMany(ctx, actor, []string{noteID})

	return err
}

// DeleteMany deletes the notes like Delete, all in one transaction, and
// returns how many it removed. A note of somebody else fails the whole call.
func (s *Service) DeleteMany(ctx context.Context, actor *model.User, noteIDs []string) (int, error) {
	deleted := 0

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		deleted = 0

		for _, id := range sortedUnique(noteIDs) {
			if actor != nil {
				if _, err := tx.NoteByID(ctx, id); errors.Is(err, repo.ErrNotFound) {
					continue
				} else if err != nil {
					return err
				}
			}

			note, err := s.ownNote(ctx, tx, actor, id, write)
			if err != nil {
				return err
			}

			if note.RecordingID != nil {
				if s.jobs == nil {
					return errors.New("notes: no job queue to delete a recording with")
				}

				if err := tx.Enqueue(ctx, s.jobs, DeleteAudioArgs{RecordingID: *note.RecordingID}); err != nil {
					return err
				}
			}

			if err := tx.DeleteNote(ctx, id); err != nil {
				return err
			}

			deleted++
		}

		return nil
	})

	return deleted, err
}

// MarkUsed records that a note of the actor was used in a post of the
// actor. Marking a pair again keeps the first record.
func (s *Service) MarkUsed(ctx context.Context, actor *model.User, noteID, postID string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if _, err := s.ownNote(ctx, tx, actor, noteID, read); err != nil {
			return err
		}

		post, err := tx.PostByID(ctx, postID)
		if errors.Is(err, repo.ErrNotFound) {
			return service.ErrNotFound
		} else if err != nil {
			return err
		}

		if post.UserID != actor.ID {
			return service.ErrNotFound
		}

		return tx.LinkNotePost(ctx, noteID, postID, model.NotePostLinkInserted)
	})
}

// ownNotes loads the notes of the actor, locked for update when write is
// set, in a fixed order so that two bulk changes can't deadlock. One that
// isn't the actor's, or doesn't exist, fails the call.
func (s *Service) ownNotes(ctx context.Context, tx *repo.Store, actor *model.User, ids []string, forWrite bool) ([]*model.Note, error) {
	a := read
	if forWrite {
		a = write
	}

	ids = sortedUnique(ids)
	notes := make([]*model.Note, 0, len(ids))

	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	for _, id := range ids {
		n, err := s.ownNote(ctx, tx, actor, id, a)
		if err != nil {
			return nil, err
		}

		notes = append(notes, n)
	}

	return notes, nil
}

func sortedUnique(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)

	return slices.Compact(out)
}
