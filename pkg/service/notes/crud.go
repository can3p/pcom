package notes

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// DeleteAudioArgs is the job deleting a note's recording from the bucket,
// enqueued in the transaction that deletes the note.
type DeleteAudioArgs struct {
	UploadID string `json:"upload_id"`
}

// Kind names the job kind.
func (DeleteAudioArgs) Kind() string { return "notes.delete_audio" }

// Input is a new note.
type Input struct {
	Body string
	// Tags are set by an action; the #words of Body are tags anyway.
	Tags    []string
	Source  *SourceInput
	Locator string
	// CapturedAt is when the note was taken; the zero value means now.
	CapturedAt time.Time
	// ApplyFocus adds the actor's focus tag, if the focus has not expired,
	// and renews it.
	ApplyFocus bool
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
		tags := in.Tags

		if in.ApplyFocus {
			focus, err := s.focus(ctx, tx, actor.ID)
			if err != nil {
				return err
			}

			if focus != "" {
				tags = append(slices.Clone(tags), focus)
			}
		}

		set, text, err := s.tagsOf(in.Body, tags)
		if err != nil && in.ApplyFocus && len(in.Tags) < len(tags) {
			// The focus tag may not fail a capture that is fine without it.
			set, text, err = s.tagsOf(in.Body, in.Tags)
		}

		if err != nil {
			return err
		}

		note = &model.Note{ID: id.String(), UserID: actor.ID, Body: in.Body, Tags: set, TextTags: text, CapturedAt: in.CapturedAt}
		if note.CapturedAt.IsZero() {
			note.CapturedAt = s.now()
		}

		if in.Locator != "" {
			note.Locator = &in.Locator
		}

		if in.Source != nil {
			src, err := s.resolveSource(ctx, tx, actor, *in.Source)
			if err != nil {
				return err
			}

			note.SourceID, note.Source = &src.ID, src
		}

		if err := tx.CreateNote(ctx, note); err != nil {
			return err
		}

		if in.ApplyFocus {
			return s.touchFocus(ctx, tx, actor.ID)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// Edit is a change of a note; a nil field keeps what the note has. Tags
// replace the ones set by actions; the ones from the text follow Body.
type Edit struct {
	Body *string
	Tags *[]string
	// Locator is where in the source the note comes from; "" clears it.
	Locator *string
	// Source gives the note the source with this title and author, found or
	// created like a capture's (a book when Kind is empty); an empty Title
	// takes the note's source away.
	Source *SourceInput
}

// Update changes a note of the actor.
func (s *Service) Update(ctx context.Context, actor *model.User, noteID string, in Edit) (*model.Note, error) {
	var note *model.Note

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		n, err := s.ownNote(ctx, tx, actor, noteID, write)
		if err != nil {
			return err
		}

		body, tags := n.Body, n.Tags

		if in.Body != nil {
			body = *in.Body
		}

		if in.Tags != nil {
			tags = *in.Tags
		}

		n.Body = body
		if n.Tags, n.TextTags, err = s.tagsOf(body, tags); err != nil {
			return err
		}

		if in.Locator != nil {
			n.Locator = nil
			if *in.Locator != "" {
				n.Locator = in.Locator
			}
		}

		if in.Source != nil {
			if err := s.setSourceOf(ctx, tx, actor, n, *in.Source); err != nil {
				return err
			}
		}

		note = n

		return tx.SaveNote(ctx, n, "body", "tags", "text_tags", "locator", "source_id", "updated_at")
	})
	if err != nil {
		return nil, err
	}

	return note, nil
}

// setSourceOf points n at the actor's source in, or at none when its title
// is empty.
func (s *Service) setSourceOf(ctx context.Context, tx *repo.Store, actor *model.User, n *model.Note, in SourceInput) error {
	if strings.TrimSpace(in.Title) == "" {
		n.SourceID, n.Source = nil, nil

		return nil
	}

	if in.Kind == "" {
		in.Kind = model.NoteSourceBook
	}

	src, err := s.resolveSource(ctx, tx, actor, in)
	if err != nil {
		return err
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

			if note.AudioUploadID != nil {
				if s.jobs == nil {
					return errors.New("notes: no job queue to delete a recording with")
				}

				if err := tx.Enqueue(ctx, s.jobs, DeleteAudioArgs{UploadID: *note.AudioUploadID}); err != nil {
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
