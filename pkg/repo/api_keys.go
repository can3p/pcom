package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// UserByAPIKey returns the user an API key belongs to, or ErrNotFound.
func (s *Store) UserByAPIKey(ctx context.Context, key string) (*core.User, error) {
	k, err := core.UserAPIKeys(core.UserAPIKeyWhere.APIKey.EQ(key)).One(ctx, s.exec)
	if err != nil {
		return nil, notFound(err)
	}

	return s.UserByID(ctx, k.UserID)
}

// APIKeyForUser returns the user's API key, or nil when they have none.
func (s *Store) APIKeyForUser(ctx context.Context, userID string) (*core.UserAPIKey, error) {
	k, err := core.UserAPIKeys(core.UserAPIKeyWhere.UserID.EQ(userID)).One(ctx, s.exec)
	if err = notFound(err); err == ErrNotFound {
		return nil, nil
	}

	return k, err
}

// CreateAPIKey gives the user an API key. A user has at most one, and
// there is no rotation: creating again keeps the existing key.
func (s *Store) CreateAPIKey(ctx context.Context, userID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	key, err := uuid.NewV7()
	if err != nil {
		return err
	}

	record := core.UserAPIKey{ID: id.String(), APIKey: key.String(), UserID: userID}

	return record.Upsert(ctx, s.exec, false, []string{core.UserAPIKeyColumns.UserID}, boil.Infer(), boil.Infer())
}
