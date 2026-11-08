package notes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// SourceCount is a source and how many notes come from it.
type SourceCount = repo.NoteSourceCount

// SourceInput names a source. The same title and author, whatever the case
// and the spacing, is the same source.
type SourceInput struct {
	Kind   model.NoteSourceKind
	Title  string
	Author string
	URL    string
}

// validSource checks the input and returns the source it makes, without ID
// and owner.
func (s *Service) validSource(in SourceInput) (*model.NoteSource, error) {
	title := strings.Join(strings.Fields(in.Title), " ")

	switch {
	case title == "":
		return nil, service.Invalid("title", "A source needs a title.")
	case utf8.RuneCountInString(title) > s.limits.SourceTitleMaxLength:
		return nil, service.Invalid("title", fmt.Sprintf("A title can be at most %d characters long.", s.limits.SourceTitleMaxLength))
	case in.Kind != model.NoteSourceBook && in.Kind != model.NoteSourceWeb:
		return nil, service.Invalid("kind", "A source is a book or a web page.")
	}

	src := &model.NoteSource{Kind: in.Kind, Title: title, MatchKey: model.NoteSourceMatchKey(title, in.Author)}

	if author := strings.Join(strings.Fields(in.Author), " "); author != "" {
		src.Author = &author
	}

	if u := strings.TrimSpace(in.URL); u != "" {
		src.URL = &u
	}

	return src, nil
}

// ownSource loads a source of the actor; one of somebody else doesn't exist.
func (s *Service) ownSource(ctx context.Context, store *repo.Store, actor *model.User, id string) (*model.NoteSource, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
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

// resolveSource finds the actor's source with that title and author or
// creates it.
func (s *Service) resolveSource(ctx context.Context, tx *repo.Store, actor *model.User, in SourceInput) (*model.NoteSource, error) {
	src, err := s.validSource(in)
	if err != nil {
		return nil, err
	}

	if found, err := tx.NoteSourceByMatchKey(ctx, actor.ID, src.MatchKey); err != nil || found != nil {
		return found, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	src.ID, src.UserID = id.String(), actor.ID

	return tx.CreateNoteSource(ctx, src)
}

// Sources returns the actor's sources that have notes, by title.
func (s *Service) Sources(ctx context.Context, actor *model.User) ([]SourceCount, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.NoteSourcesWithNotes(ctx, actor.ID)
}

// SetSource gives the notes a source, found or created by its title and
// author; nil takes their source away. One note that isn't the actor's fails
// the whole call.
func (s *Service) SetSource(ctx context.Context, actor *model.User, noteIDs []string, in *SourceInput) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		notes, err := s.ownNotes(ctx, tx, actor, noteIDs, true)
		if err != nil || len(notes) == 0 {
			return err
		}

		var sourceID *string

		if in != nil {
			src, err := s.resolveSource(ctx, tx, actor, *in)
			if err != nil {
				return err
			}

			sourceID = &src.ID
		}

		return tx.SetNotesSource(ctx, sortedUnique(noteIDs), sourceID)
	})
}

// RenameSource changes the title, author, URL and kind of a source. Naming
// it like another source of the actor is a conflict: merge them instead.
func (s *Service) RenameSource(ctx context.Context, actor *model.User, sourceID string, in SourceInput) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		src, err := s.ownSource(ctx, tx, actor, sourceID)
		if err != nil {
			return err
		}

		changed, err := s.validSource(in)
		if err != nil {
			return err
		}

		other, err := tx.NoteSourceByMatchKey(ctx, actor.ID, changed.MatchKey)
		if err != nil {
			return err
		}

		if other != nil && other.ID != src.ID {
			return fmt.Errorf("%w: there is a source with this title and author already", service.ErrConflict)
		}

		src.Kind, src.Title, src.Author, src.URL, src.MatchKey = changed.Kind, changed.Title, changed.Author, changed.URL, changed.MatchKey

		return tx.SaveNoteSource(ctx, src)
	})
}

// MergeSources moves the notes of the sources to into and deletes them.
func (s *Service) MergeSources(ctx context.Context, actor *model.User, fromIDs []string, into string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if _, err := s.ownSource(ctx, tx, actor, into); err != nil {
			return err
		}

		from := slices.DeleteFunc(sortedUnique(fromIDs), func(id string) bool { return id == into })

		for _, id := range from {
			if _, err := s.ownSource(ctx, tx, actor, id); err != nil {
				return err
			}
		}

		if len(from) == 0 {
			return nil
		}

		if err := tx.MoveNotesSource(ctx, from, into); err != nil {
			return err
		}

		for _, id := range from {
			if err := tx.DeleteNoteSource(ctx, id); err != nil {
				return err
			}
		}

		return nil
	})
}
