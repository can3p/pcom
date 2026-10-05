package posts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
)

// promptTimeout is how long a user has to wait between two prompts.
const promptTimeout = 5 * time.Minute

func wrapPrompt(prompt *model.PostPrompt, err error) (*postops.PostPrompt, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &postops.PostPrompt{Prompt: prompt, Author: prompt.Asker}, nil
}

// promptForRecipient is the prompt addressed to recipientID, nil if there is
// none.
func promptForRecipient(ctx context.Context, store *repo.Store, recipientID, id string) (*postops.PostPrompt, error) {
	prompt, err := store.PromptForRecipient(ctx, recipientID, id)

	return wrapPrompt(prompt, err)
}

// promptByPost is the prompt a post answers, nil if there is none.
func promptByPost(ctx context.Context, store *repo.Store, postID string) (*postops.PostPrompt, error) {
	prompt, err := store.PromptForPost(ctx, postID)

	return wrapPrompt(prompt, err)
}

// PromptFor returns the prompt with the given ID if it is addressed to the
// actor, and nil otherwise.
func (s *Service) PromptFor(ctx context.Context, actor *model.User, promptID string) (*postops.PostPrompt, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	return promptForRecipient(ctx, s.store, actor.ID, promptID)
}

// DirectConnections lists the actor's direct connections, the users they may
// prompt.
func (s *Service) DirectConnections(ctx context.Context, actor *model.User) ([]*model.User, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	ids, err := graph.DirectUserIDs(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	return s.store.UsersByIDs(ctx, ids)
}

// CanPrompt reports whether the actor may send a prompt right now: prompts
// are rate limited. The error reads as a sentence.
func (s *Service) CanPrompt(ctx context.Context, actor *model.User) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	return canPrompt(ctx, s.store, actor)
}

func canPrompt(ctx context.Context, store *repo.Store, actor *model.User) error {
	last, err := store.LastPromptBy(ctx, actor.ID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	if time.Since(last.CreatedAt) > promptTimeout {
		return nil
	}

	return service.Invalid("", fmt.Sprintf("you cannot send prompts for another %s", util.FormatDuration(time.Until(last.CreatedAt.Add(promptTimeout)))))
}

// checkPrompt enforces the length of a prompt message.
func (s *Service) checkPrompt(message string) error {
	if err := validation.ValidateMinMax("message", message, PromptMinLength, s.limits.PromptMaxLength); err != nil {
		return service.Invalid("message", err.Error())
	}

	return nil
}

// SendPrompt asks a user to write a post on a subject, and tells them by
// mail. The recipient must be a direct connection of the actor; anyone else
// is refused with the wording the prompt form always used.
func (s *Service) SendPrompt(ctx context.Context, actor, recipient *model.User, message string) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	if err := s.checkPrompt(message); err != nil {
		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		radius, err := graph.RadiusBetween(ctx, tx, actor.ID, recipient.ID)
		if err != nil {
			return err
		}

		if radius != graph.RadiusDirect {
			return service.Invalid("", fmt.Sprintf("'%s' is not your direct connection", recipient.Username))
		}

		if err := canPrompt(ctx, tx, actor); err != nil {
			return err
		}

		id, err := uuid.NewV7()
		if err != nil {
			return err
		}

		prompt := &model.PostPrompt{
			ID:          id.String(),
			AskerID:     actor.ID,
			Message:     strings.TrimSpace(message),
			RecipientID: recipient.ID,
		}

		if err := tx.InsertPrompt(ctx, prompt); err != nil {
			return err
		}

		return s.queue(ctx, tx, mail.PostPrompt(s.ident.Site, s.ident.From, actor, recipient, prompt))
	})
}

// DismissPrompt hides a prompt addressed to the actor. What the actor sees
// when there is no such prompt is the database's own words, as it always was.
func (s *Service) DismissPrompt(ctx context.Context, actor *model.User, promptID string) error {
	if err := requireActor(actor); err != nil {
		return err
	}

	err := s.store.DismissPrompt(ctx, actor.ID, promptID, time.Now())
	if errors.Is(err, repo.ErrNotFound) {
		return service.Invalid("promptId", "sql: no rows in result set")
	}

	return err
}
