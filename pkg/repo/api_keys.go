package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

// UserByAPIKey returns the user an API key belongs to, or ErrNotFound.
func (s *Store) UserByAPIKey(ctx context.Context, key string) (*model.User, error) {
	k := new(model.UserAPIKey)
	err := s.query().NewSelect().Model(k).Where("api_key = ?", key).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return s.UserByID(ctx, k.UserID)
}

// APIKeyForUser returns the user's API key, or nil when they have none.
func (s *Store) APIKeyForUser(ctx context.Context, userID string) (*model.UserAPIKey, error) {
	k := new(model.UserAPIKey)
	err := s.query().NewSelect().Model(k).Where("user_id = ?", userID).Limit(1).Scan(ctx)

	return orNil(k, err)
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

	record := &model.UserAPIKey{ID: id.String(), APIKey: key.String(), UserID: userID}

	_, err = s.query().NewInsert().Model(record).On("CONFLICT (user_id) DO NOTHING").Exec(ctx)

	return err
}
