package notes

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

// Attaching a note to a post changes no text, unlike inserting it: the note
// stays the author's alone and the post's readers never see it. A note
// belongs to a post when it is attached one by one or carries a tag attached
// to the post, now or later, unless it is excluded from the post. Detaching,
// excluding, deleting the post or the note removes links only; a post never
// deletes a note. Every method needs the actor to own the post and the note
// it names, and anybody else is told they don't exist.

// AttachedNote is a note that belongs to a post: Attached says it was
// attached one by one, ViaTags are the post's attached tags it carries.
type AttachedNote = repo.PostNoteRow

// PostNotes are the notes that belong to a post, newest capture first, and
// the tags attached to it, by name.
type PostNotes struct {
	Notes []AttachedNote
	Tags  []string
}

// Attach attaches a note of the actor to a post of the actor, taking back
// its exclusion if it had one. Attaching it again changes nothing.
func (s *Service) Attach(ctx context.Context, actor *model.User, noteID, postID string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := s.ownNoteAndPost(ctx, tx, actor, noteID, postID); err != nil {
			return err
		}

		if err := tx.UnlinkNotePost(ctx, noteID, postID, model.NotePostLinkExcluded); err != nil {
			return err
		}

		return tx.LinkNotePost(ctx, noteID, postID, model.NotePostLinkAttached)
	})
}

// Detach takes back the attaching of a note one by one; a note that carries
// one of the post's tags still belongs to it.
func (s *Service) Detach(ctx context.Context, actor *model.User, noteID, postID string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := s.ownNoteAndPost(ctx, tx, actor, noteID, postID); err != nil {
			return err
		}

		return tx.UnlinkNotePost(ctx, noteID, postID, model.NotePostLinkAttached)
	})
}

// Exclude keeps a note out of a post although it carries one of the post's
// tags; a note attached one by one is detached as well. The note keeps its
// tags.
func (s *Service) Exclude(ctx context.Context, actor *model.User, noteID, postID string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := s.ownNoteAndPost(ctx, tx, actor, noteID, postID); err != nil {
			return err
		}

		if err := tx.UnlinkNotePost(ctx, noteID, postID, model.NotePostLinkAttached); err != nil {
			return err
		}

		return tx.LinkNotePost(ctx, noteID, postID, model.NotePostLinkExcluded)
	})
}

// AttachTag attaches a tag to a post of the actor: the actor's notes that
// carry it now, and every note tagged with it later, belong to the post. It
// returns how many notes the tag brings now, those excluded left out.
func (s *Service) AttachTag(ctx context.Context, actor *model.User, postID, tag string) (int, error) {
	count := 0

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		if _, err := s.ownPost(ctx, tx, actor, postID); err != nil {
			return err
		}

		tag, err := s.normalizeTag("tag", tag)
		if err != nil {
			return err
		}

		if err := tx.AddPostNoteTag(ctx, postID, tag); err != nil {
			return err
		}

		count, err = tx.CountTaggedNotesOfPost(ctx, actor.ID, postID, tag)

		return err
	})

	return count, err
}

// DetachTag detaches a tag from a post of the actor; the notes it brought no
// longer belong to the post unless something else attaches them.
func (s *Service) DetachTag(ctx context.Context, actor *model.User, postID, tag string) error {
	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if _, err := s.ownPost(ctx, tx, actor, postID); err != nil {
			return err
		}

		tag, err := s.normalizeTag("tag", tag)
		if err != nil {
			return err
		}

		return tx.RemovePostNoteTag(ctx, postID, tag)
	})
}

// ForPost returns the notes that belong to a post of the actor, and its
// attached tags.
func (s *Service) ForPost(ctx context.Context, actor *model.User, postID string) (*PostNotes, error) {
	post, err := s.ownPost(ctx, s.store, actor, postID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.PostNoteRows(ctx, post.UserID, post.ID)
	if err != nil {
		return nil, err
	}

	tags, err := s.store.PostNoteTags(ctx, post.ID)
	if err != nil {
		return nil, err
	}

	return &PostNotes{Notes: rows, Tags: tags}, nil
}

// Post returns a post of the actor, which the page of its notes names.
func (s *Service) Post(ctx context.Context, actor *model.User, postID string) (*model.Post, error) {
	return s.ownPost(ctx, s.store, actor, postID)
}

// PostsOf returns the posts a note of the actor belongs to, newest first.
func (s *Service) PostsOf(ctx context.Context, actor *model.User, noteID string) ([]*model.Post, error) {
	note, err := s.ownNote(ctx, s.store, actor, noteID, read)
	if err != nil {
		return nil, err
	}

	return s.store.PostsOfNote(ctx, note.UserID, note.ID)
}

func (s *Service) ownNoteAndPost(ctx context.Context, tx *repo.Store, actor *model.User, noteID, postID string) error {
	if _, err := s.ownNote(ctx, tx, actor, noteID, read); err != nil {
		return err
	}

	_, err := s.ownPost(ctx, tx, actor, postID)

	return err
}

// ownPost returns a post of the actor; an anonymous actor needs to log in,
// and a post of somebody else doesn't exist for them.
func (s *Service) ownPost(ctx context.Context, store *repo.Store, actor *model.User, postID string) (*model.Post, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	post, err := store.PostByID(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if post.UserID != actor.ID {
		return nil, service.ErrNotFound
	}

	return post, nil
}

// PostsOfNotes returns the posts each of the actor's notes among noteIDs
// belongs to, newest first, in one read; a note that belongs to none, or
// isn't the actor's, has no entry.
func (s *Service) PostsOfNotes(ctx context.Context, actor *model.User, noteIDs []string) (map[string][]*model.Post, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.PostsOfNotes(ctx, actor.ID, noteIDs)
}

// AttachablePosts returns the actor's newest saved drafts and posts, as many
// as Limits.AttachPosts says, which the Attach to post control offers.
func (s *Service) AttachablePosts(ctx context.Context, actor *model.User) ([]*model.Post, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.RecentPosts(ctx, actor.ID, s.limits.AttachPosts)
}
