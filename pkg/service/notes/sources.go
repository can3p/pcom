package notes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// SourceCount is a source and how many notes come from it.
type SourceCount = repo.NoteSourceCount

// validTitle checks a source's title and returns it with its whitespace
// collapsed.
func (s *Service) validTitle(title string) (string, error) {
	title = strings.Join(strings.Fields(title), " ")

	switch {
	case title == "":
		return "", service.Invalid("title", "A source needs a title.")
	case utf8.RuneCountInString(title) > s.limits.SourceTitleMaxLength:
		return "", service.Invalid("title", fmt.Sprintf("A title can be at most %d characters long.", s.limits.SourceTitleMaxLength))
	}

	return title, nil
}

// ownSource loads a source of the actor; one of somebody else doesn't exist.
func (s *Service) ownSource(ctx context.Context, store *repo.Store, actor *model.User, id string) (*model.NoteSource, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	if uuid.Validate(id) != nil {
		return nil, service.ErrNotFound
	}

	src, err := store.NoteSourceByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if src.UserID != actor.ID {
		return nil, service.ErrNotFound
	}

	return src, nil
}

// resolveSource finds the actor's source with that title, whatever its case
// and spacing, or creates it.
func (s *Service) resolveSource(ctx context.Context, tx *repo.Store, actor *model.User, title string) (*model.NoteSource, error) {
	title, err := s.validTitle(title)
	if err != nil {
		return nil, err
	}

	key := model.NoteSourceMatchKey(title)

	if found, err := tx.NoteSourceByMatchKey(ctx, actor.ID, key); err != nil || found != nil {
		return found, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return tx.CreateNoteSource(ctx, &model.NoteSource{ID: id.String(), UserID: actor.ID, Title: title, MatchKey: key})
}

// Sources returns all the actor's sources, by title, with how many notes
// come from each.
func (s *Service) Sources(ctx context.Context, actor *model.User) ([]SourceCount, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.NoteSourceCounts(ctx, actor.ID)
}

// AddSource returns the actor's source with that title, made when there is
// none yet.
func (s *Service) AddSource(ctx context.Context, actor *model.User, title string) (*model.NoteSource, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	var src *model.NoteSource

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		var err error
		src, err = s.resolveSource(ctx, tx, actor, title)

		return err
	})

	return src, err
}

// Source returns a source of the actor.
func (s *Service) Source(ctx context.Context, actor *model.User, id string) (*model.NoteSource, error) {
	return s.ownSource(ctx, s.store, actor, id)
}

// SetSource gives the notes the source with that title, found or created; an
// empty title takes their source away. One note that isn't the actor's fails
// the whole call.
func (s *Service) SetSource(ctx context.Context, actor *model.User, noteIDs []string, title string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		notes, err := s.ownNotes(ctx, tx, actor, noteIDs, true)
		if err != nil || len(notes) == 0 {
			return err
		}

		var sourceID *string

		if strings.TrimSpace(title) != "" {
			src, err := s.resolveSource(ctx, tx, actor, title)
			if err != nil {
				return err
			}

			sourceID = &src.ID
		}

		return tx.SetNotesSource(ctx, sortedUnique(noteIDs), sourceID)
	})
}

// RenameSource gives a source of the actor a new title and returns the
// source its notes now come from. Renaming it to the title of another of the
// actor's sources merges the two, as renaming a tag does: its notes move to
// that source, which is returned, and it is deleted.
func (s *Service) RenameSource(ctx context.Context, actor *model.User, sourceID, title string) (*model.NoteSource, error) {
	var out *model.NoteSource

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		src, err := s.ownSource(ctx, tx, actor, sourceID)
		if err != nil {
			return err
		}

		title, err := s.validTitle(title)
		if err != nil {
			return err
		}

		key := model.NoteSourceMatchKey(title)

		other, err := tx.NoteSourceByMatchKey(ctx, actor.ID, key)
		if err != nil {
			return err
		}

		if other != nil && other.ID != src.ID {
			if err := tx.MoveNotesSource(ctx, []string{src.ID}, other.ID); err != nil {
				return err
			}

			out = other

			return tx.DeleteNoteSource(ctx, src.ID)
		}

		src.Title, src.MatchKey = title, key
		out = src

		return tx.SaveNoteSource(ctx, src)
	})

	return out, err
}
