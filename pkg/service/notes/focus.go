package notes

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

// SetFocus makes tag the tag the actor's captures get; an empty tag clears
// the focus.
func (s *Service) SetFocus(ctx context.Context, actor *model.User, tag string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if tag == "" {
		return s.store.SetNotesFocus(ctx, actor.ID, nil, nil)
	}

	tag, err := s.normalizeTag("tag", tag)
	if err != nil {
		return err
	}

	return s.store.SetNotesFocus(ctx, actor.ID, &tag, new(s.now()))
}

// Focus returns the actor's focus tag, or "" when they have none or it has
// been idle for the focus idle time.
func (s *Service) Focus(ctx context.Context, actor *model.User) (string, error) {
	if actor == nil {
		return "", service.ErrNeedsLogin
	}

	return s.focus(ctx, s.store, actor.ID)
}

// TouchFocus renews a focus that hasn't expired: a capture under it keeps it
// from going idle. An expired or absent focus stays so.
func (s *Service) TouchFocus(ctx context.Context, actor *model.User) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	return s.touchFocus(ctx, s.store, actor.ID)
}

func (s *Service) focus(ctx context.Context, store *repo.Store, userID string) (string, error) {
	u, err := store.UserByID(ctx, userID)
	if err != nil {
		return "", err
	}

	if u.NotesFocusTag == nil || u.NotesFocusAt == nil || s.now().Sub(*u.NotesFocusAt) > s.limits.FocusIdle {
		return "", nil
	}

	return *u.NotesFocusTag, nil
}

func (s *Service) touchFocus(ctx context.Context, store *repo.Store, userID string) error {
	tag, err := s.focus(ctx, store, userID)
	if err != nil || tag == "" {
		return err
	}

	return store.SetNotesFocus(ctx, userID, &tag, new(s.now()))
}
