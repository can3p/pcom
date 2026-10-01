package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// ProfileAbout returns the user's "About" text, or "" when there is none.
func (s *Store) ProfileAbout(ctx context.Context, userID string) (string, error) {
	p, err := core.FindUserProfile(ctx, s.exec, userID, core.UserProfileColumns.About)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return p.About, nil
}

// SaveProfileAbout creates the user's "About" text or replaces it.
func (s *Store) SaveProfileAbout(ctx context.Context, userID, about string) error {
	p := core.UserProfile{UserID: userID, About: about, UpdatedAt: time.Now()}

	return p.Upsert(
		ctx, s.exec, true, []string{core.UserProfileColumns.UserID},
		boil.Whitelist(core.UserProfileColumns.About, core.UserProfileColumns.UpdatedAt),
		boil.Infer(),
	)
}

// DeleteProfileAbout removes the user's "About" text; no text is fine.
func (s *Store) DeleteProfileAbout(ctx context.Context, userID string) error {
	_, err := core.UserProfiles(core.UserProfileWhere.UserID.EQ(userID)).DeleteAll(ctx, s.exec)

	return err
}
