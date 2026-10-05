package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// PromptForRecipient returns a prompt addressed to the recipient, with the
// asker loaded (prompt.Asker).
func (s *Store) PromptForRecipient(ctx context.Context, recipientID, id string) (*model.PostPrompt, error) {
	prompt := new(model.PostPrompt)
	err := s.query().NewSelect().Model(prompt).
		Relation("Asker").
		Where("?TableAlias.recipient_id = ?", recipientID).
		Where("?TableAlias.id = ?", id).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return prompt, nil
}

// PromptForPost returns the prompt a post answers, with the asker loaded.
func (s *Store) PromptForPost(ctx context.Context, postID string) (*model.PostPrompt, error) {
	prompt := new(model.PostPrompt)
	err := s.query().NewSelect().Model(prompt).
		Relation("Asker").
		Where("?TableAlias.post_id = ?", postID).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return prompt, nil
}

// LastPromptBy returns the newest prompt the user sent, or ErrNotFound.
func (s *Store) LastPromptBy(ctx context.Context, askerID string) (*model.PostPrompt, error) {
	prompt := new(model.PostPrompt)
	err := s.query().NewSelect().Model(prompt).
		Where("asker_id = ?", askerID).
		OrderExpr("created_at DESC").
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return prompt, nil
}

// InsertPrompt stores a new prompt.
func (s *Store) InsertPrompt(ctx context.Context, prompt *model.PostPrompt) error {
	_, err := s.query().NewInsert().Model(prompt).Exec(ctx)
	return err
}

// UpdatePrompt writes every column of an existing prompt.
func (s *Store) UpdatePrompt(ctx context.Context, prompt *model.PostPrompt) error {
	_, err := s.query().NewUpdate().Model(prompt).WherePK().Exec(ctx)
	return err
}

// DismissPrompt marks a prompt addressed to the recipient as dismissed at the
// given time.
func (s *Store) DismissPrompt(ctx context.Context, recipientID, id string, at time.Time) error {
	prompt := new(model.PostPrompt)

	err := s.query().NewSelect().Model(prompt).
		Where("recipient_id = ?", recipientID).
		Where("id = ?", id).
		Limit(1).Scan(ctx)
	if err != nil {
		return notFound(err)
	}

	prompt.DismissedAt = &at

	_, err = s.query().NewUpdate().Model(prompt).WherePK().Exec(ctx)

	return err
}

// OpenPromptsFor returns the prompts sent to recipientID that they haven't
// dismissed, newest first, with the asker and the answer post loaded.
func (s *Store) OpenPromptsFor(ctx context.Context, recipientID string) ([]*model.PostPrompt, error) {
	var prompts []*model.PostPrompt
	err := s.query().NewSelect().Model(&prompts).
		Relation("Asker").
		Relation("Post").
		Where("?TableAlias.recipient_id = ?", recipientID).
		Where("?TableAlias.dismissed_at IS NULL").
		OrderExpr("?TableAlias.created_at DESC").
		Scan(ctx)

	return prompts, err
}
