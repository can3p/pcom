package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// RegistrationOpen reports the system setting that lets anybody sign up.
func (s *Store) RegistrationOpen(ctx context.Context) (bool, error) {
	settings, err := core.SystemSettings().One(ctx, s.exec)
	if err != nil {
		return false, err
	}

	return settings.RegistrationOpen, nil
}

// SetRegistrationOpen flips the system setting that lets anybody sign up.
func (s *Store) SetRegistrationOpen(ctx context.Context, open bool) error {
	settings, err := core.SystemSettings().One(ctx, s.exec)
	if err != nil {
		return err
	}

	settings.RegistrationOpen = open
	_, err = settings.Update(ctx, s.exec, boil.Whitelist(core.SystemSettingColumns.RegistrationOpen))

	return err
}

// UserStyleForUser returns the user's custom CSS row, or nil when they have
// none.
func (s *Store) UserStyleForUser(ctx context.Context, userID string) (*core.UserStyle, error) {
	st, err := core.UserStyles(core.UserStyleWhere.UserID.EQ(userID)).One(ctx, s.exec)
	if err = notFound(err); err == ErrNotFound {
		return nil, nil
	}

	return st, err
}

// UserStyleByUsername returns the custom CSS row of the user with the
// username, or nil when the user or the row does not exist.
func (s *Store) UserStyleByUsername(ctx context.Context, username string) (*core.UserStyle, error) {
	user, err := core.Users(core.UserWhere.Username.EQ(username)).One(ctx, s.exec)
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

	style := core.UserStyle{ID: id.String(), UserID: userID, Styles: styles}

	return style.Upsert(
		ctx, s.exec, true, []string{core.UserStyleColumns.UserID},
		boil.Whitelist(core.UserStyleColumns.UpdatedAt, core.UserStyleColumns.Styles),
		boil.Infer(),
	)
}
