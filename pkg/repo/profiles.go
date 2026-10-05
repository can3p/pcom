package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// ProfileAbout returns the user's "About" text, or "" when there is none.
func (s *Store) ProfileAbout(ctx context.Context, userID string) (string, error) {
	p := new(model.UserProfile)
	err := s.query().NewSelect().Model(p).Column("about").Where("user_id = ?", userID).Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return p.About, nil
}

// SaveProfileAbout creates the user's "About" text or replaces it.
func (s *Store) SaveProfileAbout(ctx context.Context, userID, about string) error {
	p := &model.UserProfile{UserID: userID, About: about, UpdatedAt: time.Now()}

	_, err := s.query().NewInsert().Model(p).
		On("CONFLICT (user_id) DO UPDATE").
		Set("about = EXCLUDED.about").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)

	return err
}

// DeleteProfileAbout removes the user's "About" text; no text is fine.
func (s *Store) DeleteProfileAbout(ctx context.Context, userID string) error {
	_, err := s.query().NewDelete().Model((*model.UserProfile)(nil)).Where("user_id = ?", userID).Exec(ctx)

	return err
}
