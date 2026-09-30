package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// PromptForRecipient returns a prompt addressed to the recipient, with the
// asker loaded (prompt.R.Asker).
func (s *Store) PromptForRecipient(ctx context.Context, recipientID, id string) (*core.PostPrompt, error) {
	prompt, err := core.PostPrompts(
		qm.Load(core.PostPromptRels.Asker),
		core.PostPromptWhere.RecipientID.EQ(recipientID),
		core.PostPromptWhere.ID.EQ(id),
	).One(ctx, s.exec)

	return prompt, notFound(err)
}

// PromptForPost returns the prompt a post answers, with the asker loaded.
func (s *Store) PromptForPost(ctx context.Context, postID string) (*core.PostPrompt, error) {
	prompt, err := core.PostPrompts(
		qm.Load(core.PostPromptRels.Asker),
		core.PostPromptWhere.PostID.EQ(null.StringFrom(postID)),
	).One(ctx, s.exec)

	return prompt, notFound(err)
}

// LastPromptBy returns the newest prompt the user sent, or ErrNotFound.
func (s *Store) LastPromptBy(ctx context.Context, askerID string) (*core.PostPrompt, error) {
	prompt, err := core.PostPrompts(
		core.PostPromptWhere.AskerID.EQ(askerID),
		qm.OrderBy(core.PostPromptColumns.CreatedAt+" DESC"),
		qm.Limit(1),
	).One(ctx, s.exec)

	return prompt, notFound(err)
}

// InsertPrompt stores a new prompt.
func (s *Store) InsertPrompt(ctx context.Context, prompt *core.PostPrompt) error {
	return prompt.Insert(ctx, s.exec, boil.Infer())
}

// UpdatePrompt writes every column of an existing prompt.
func (s *Store) UpdatePrompt(ctx context.Context, prompt *core.PostPrompt) error {
	_, err := prompt.Update(ctx, s.exec, boil.Infer())
	return err
}

// DismissPrompt marks a prompt addressed to the recipient as dismissed at the
// given time.
func (s *Store) DismissPrompt(ctx context.Context, recipientID, id string, at time.Time) error {
	prompt, err := core.PostPrompts(
		core.PostPromptWhere.RecipientID.EQ(recipientID),
		core.PostPromptWhere.ID.EQ(id),
	).One(ctx, s.exec)
	if err != nil {
		return notFound(err)
	}

	prompt.DismissedAt = null.TimeFrom(at)

	_, err = prompt.Update(ctx, s.exec, boil.Infer())

	return err
}

// OpenPromptsFor returns the prompts sent to recipientID that they haven't
// dismissed, newest first, with the asker and the answer post loaded.
func (s *Store) OpenPromptsFor(ctx context.Context, recipientID string) (core.PostPromptSlice, error) {
	return core.PostPrompts(
		core.PostPromptWhere.RecipientID.EQ(recipientID),
		core.PostPromptWhere.DismissedAt.IsNull(),
		qm.Load(core.PostPromptRels.Asker),
		qm.Load(core.PostPromptRels.Post),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostPromptColumns.CreatedAt)),
	).All(ctx, s.exec)
}
