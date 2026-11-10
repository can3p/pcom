package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
)

// APIKeyByHash returns the API key whose hash is keyHash
// (model.HashAPIKey), with its user, or ErrNotFound.
func (s *Store) APIKeyByHash(ctx context.Context, keyHash string) (*model.UserAPIKey, error) {
	k := new(model.UserAPIKey)

	err := s.query().NewSelect().Model(k).Relation("User").Where("?TableAlias.key_hash = ?", keyHash).Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return k, nil
}

// APIKeysForUser returns the user's API keys, oldest first.
func (s *Store) APIKeysForUser(ctx context.Context, userID string) ([]*model.UserAPIKey, error) {
	var keys []*model.UserAPIKey

	err := s.query().NewSelect().Model(&keys).Where("user_id = ?", userID).Order("created_at", "id").Scan(ctx)

	return keys, err
}

// CreateAPIKey stores k, which carries the key's hash.
func (s *Store) CreateAPIKey(ctx context.Context, k *model.UserAPIKey) error {
	_, err := s.query().NewInsert().Model(k).Exec(ctx)

	return err
}

// DeleteAPIKey deletes the user's API key keyID; ErrNotFound when the user
// has no such key.
func (s *Store) DeleteAPIKey(ctx context.Context, userID, keyID string) error {
	res, err := s.query().NewDelete().Model((*model.UserAPIKey)(nil)).Where("id = ?", keyID).Where("user_id = ?", userID).Exec(ctx)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

// TouchAPIKey records that the key keyID was used at the time at.
func (s *Store) TouchAPIKey(ctx context.Context, keyID string, at time.Time) error {
	_, err := s.query().NewUpdate().Model((*model.UserAPIKey)(nil)).Set("last_used_at = ?", at).Where("id = ?", keyID).Exec(ctx)

	return err
}
