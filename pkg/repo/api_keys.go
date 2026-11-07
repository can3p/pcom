package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
)

// LockAPIKeys guards the check of the keys a user has before one is added.
const LockAPIKeys LockScope = "api_keys"

// UserByAPIKeyHash returns the user of the API key whose hash is keyHash
// (model.HashAPIKey), or ErrNotFound.
func (s *Store) UserByAPIKeyHash(ctx context.Context, keyHash string) (*model.User, error) {
	k := new(model.UserAPIKey)
	err := s.query().NewSelect().Model(k).Where("key_hash = ?", keyHash).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return s.UserByID(ctx, k.UserID)
}

// APIKeyForUser returns the user's oldest API key, or nil when they have none.
func (s *Store) APIKeyForUser(ctx context.Context, userID string) (*model.UserAPIKey, error) {
	k := new(model.UserAPIKey)
	err := s.query().NewSelect().Model(k).Where("user_id = ?", userID).Order("created_at", "id").Limit(1).Scan(ctx)

	return orNil(k, err)
}

// CreateAPIKey stores k, which carries the key's hash.
func (s *Store) CreateAPIKey(ctx context.Context, k *model.UserAPIKey) error {
	_, err := s.query().NewInsert().Model(k).Exec(ctx)

	return err
}
