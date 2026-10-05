package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

// RegistrationOpen reports the system setting that lets anybody sign up.
func (s *Store) RegistrationOpen(ctx context.Context) (bool, error) {
	settings := new(model.SystemSetting)
	if err := s.query().NewSelect().Model(settings).Limit(1).Scan(ctx); err != nil {
		return false, err
	}

	return settings.RegistrationOpen, nil
}

// SetRegistrationOpen flips the system setting that lets anybody sign up.
func (s *Store) SetRegistrationOpen(ctx context.Context, open bool) error {
	settings := new(model.SystemSetting)
	if err := s.query().NewSelect().Model(settings).Limit(1).Scan(ctx); err != nil {
		return err
	}

	settings.RegistrationOpen = open
	_, err := s.query().NewUpdate().Model(settings).Column("registration_open").WherePK().Exec(ctx)

	return err
}

// UserStyleForUser returns the user's custom CSS row, or nil when they have
// none.
func (s *Store) UserStyleForUser(ctx context.Context, userID string) (*model.UserStyle, error) {
	st := new(model.UserStyle)
	err := s.query().NewSelect().Model(st).Where("user_id = ?", userID).Limit(1).Scan(ctx)

	return orNil(st, err)
}

// UserStyleByUsername returns the custom CSS row of the user with the
// username, or nil when the user or the row does not exist.
func (s *Store) UserStyleByUsername(ctx context.Context, username string) (*model.UserStyle, error) {
	user := new(model.User)
	err := s.query().NewSelect().Model(user).Where("username = ?", username).Limit(1).Scan(ctx)
	if err = notFound(err); err == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return s.UserStyleForUser(ctx, user.ID)
}

// SaveUserStyle creates the user's custom CSS or replaces it.
func (s *Store) SaveUserStyle(ctx context.Context, userID, styles string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	style := &model.UserStyle{ID: id.String(), UserID: userID, Styles: styles}

	_, err = s.query().NewInsert().Model(style).
		On("CONFLICT (user_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("styles = EXCLUDED.styles").
		Exec(ctx)

	return err
}
