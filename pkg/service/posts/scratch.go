package posts

import (
	"context"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// A new post lives in the author's scratch pad until it is saved as a draft
// or published: one pad per author and prompt, so answering a prompt never
// overwrites the text of an ordinary post. Every write names the revision the
// tab last saw (SaveInput.ScratchRevision); a write against a pad that another
// tab or device changed since is a *ScratchConflict and writes nothing.

// ScratchConflict is the error of a pad write that lost to another tab or
// device: the pad changed at At since the tab last saw it. PromotedPostID is
// the post the pad became when another tab saved it as a draft or published
// it; when the pad was emptied without one, it was discarded. It is a
// service.ErrConflict.
type ScratchConflict struct {
	At             time.Time
	PromotedPostID string
}

func (e *ScratchConflict) Error() string {
	return "this new post was changed on another device or tab"
}

// Unwrap makes the conflict a service.ErrConflict.
func (e *ScratchConflict) Unwrap() error {
	return service.ErrConflict
}

// Scratch loads the actor's pad for the prompt, nil if they never wrote one.
// A pad that became a post or was discarded comes back empty
// (model.PostScratch.IsEmpty) with its revision, which the form needs to
// write it again. A prompt the actor may not answer is ignored, as on the
// write page: it is the plain pad.
func (s *Service) Scratch(ctx context.Context, actor *model.User, promptID string) (*model.PostScratch, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	key, err := scratchPromptID(ctx, s.store, actor, promptID)
	if err != nil {
		return nil, err
	}

	pad, err := s.store.ScratchOf(ctx, actor.ID, key)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}

	return pad, err
}

// scratchPromptID is the prompt that keys the actor's pad: promptID if the
// actor may answer it, otherwise none.
func scratchPromptID(ctx context.Context, store *repo.Store, actor *model.User, promptID string) (string, error) {
	if promptID == "" {
		return "", nil
	}

	prompt, err := promptForRecipient(ctx, store, actor.ID, promptID)
	if err != nil || prompt == nil {
		return "", err
	}

	return prompt.Prompt.ID, nil
}

// saveScratch writes, takes over or discards the actor's pad, as the
// conflict table of SaveInput.ScratchRevision says.
func (s *Service) saveScratch(ctx context.Context, tx *repo.Store, actor *model.User, in SaveInput, action Action) (*Saved, error) {
	promptID, err := scratchPromptID(ctx, tx, actor, in.PromptID)
	if err != nil {
		return nil, err
	}

	if action == ActionDelete {
		return s.discardScratch(ctx, tx, actor, promptID, in.ScratchRevision)
	}

	pad := &model.PostScratch{
		UserID:            actor.ID,
		PromptID:          lo.EmptyableToPtr(promptID),
		Subject:           lo.EmptyableToPtr(in.Subject),
		URL:               lo.EmptyableToPtr(in.URL),
		Body:              in.Body,
		Visibility:        in.Visibility,
		NotifySubscribers: in.NotifySubscribers,
		NoteIDs:           []string{},
	}

	written := true

	switch {
	case action == ActionTakeOver:
		if err := newScratchID(pad); err != nil {
			return nil, err
		}

		err = tx.TakeOverScratch(ctx, pad)
	case in.ScratchRevision == 0:
		if err := newScratchID(pad); err != nil {
			return nil, err
		}

		written, err = tx.InsertScratch(ctx, pad)
	default:
		written, err = tx.UpdateScratch(ctx, pad, in.ScratchRevision)
	}

	if err != nil {
		return nil, err
	}

	if !written {
		return nil, scratchConflict(ctx, tx, actor.ID, promptID)
	}

	return &Saved{Scratch: pad}, nil
}

// discardScratch empties the actor's pad if it is still at revision. A pad
// never written has nothing to discard.
func (s *Service) discardScratch(ctx context.Context, tx *repo.Store, actor *model.User, promptID string, revision int) (*Saved, error) {
	emptied, err := tx.EmptyScratch(ctx, actor.ID, promptID, revision, nil, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if !emptied {
		_, err := tx.ScratchOf(ctx, actor.ID, promptID)
		if err == nil {
			return nil, scratchConflict(ctx, tx, actor.ID, promptID)
		} else if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}

	return &Saved{Deleted: true}, nil
}

// promoteScratch empties the pad a new post was made from, if it is still
// at the revision the tab saw; otherwise another tab's text stays in it.
func promoteScratch(ctx context.Context, tx *repo.Store, actor *model.User, promptID string, revision int, postID string) error {
	_, err := tx.EmptyScratch(ctx, actor.ID, promptID, revision, &postID, time.Now().UTC())

	return err
}

// scratchConflict describes the pad that a write lost to.
func scratchConflict(ctx context.Context, tx *repo.Store, userID, promptID string) error {
	pad, err := tx.ScratchOf(ctx, userID, promptID)
	if errors.Is(err, repo.ErrNotFound) {
		return &ScratchConflict{}
	} else if err != nil {
		return err
	}

	return &ScratchConflict{At: pad.UpdatedAt, PromotedPostID: lo.FromPtr(pad.PromotedPostID)}
}

func newScratchID(pad *model.PostScratch) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	pad.ID = id.String()

	return nil
}
