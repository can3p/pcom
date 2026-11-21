package accounts

import (
	"context"
	"crypto/rand"
	"strings"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
)

// CreateAPIKey gives the actor a new API key with the given name and scope
// ("" is full) and returns it with the key in plain in APIKey, the one time
// it is shown: only its hash is stored. A user may have any number of keys.
func (s *Service) CreateAPIKey(ctx context.Context, actor *model.User, name string, scope model.APIKeyScope) (*model.UserAPIKey, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	switch scope {
	case "":
		scope = model.APIKeyScopeFull
	case model.APIKeyScopeFull, model.APIKeyScopeNotes:
	default:
		return nil, service.Invalid("scope", "Choose what the key may do.")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	plain := rand.Text()
	k := &model.UserAPIKey{
		ID:      id.String(),
		KeyHash: model.HashAPIKey(plain),
		UserID:  actor.ID,
		Name:    strings.TrimSpace(name),
		Scope:   scope,
	}

	if err := s.store.CreateAPIKey(ctx, k); err != nil {
		return nil, err
	}

	k.APIKey = plain

	return k, nil
}

// APIKeys returns the actor's API keys, oldest first. They carry no key in
// plain: it is shown only by CreateAPIKey.
func (s *Service) APIKeys(ctx context.Context, actor *model.User) ([]*model.UserAPIKey, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.APIKeysForUser(ctx, actor.ID)
}

// RevokeAPIKey deletes the actor's API key keyID; it stops working at once.
// Another user's key is not found.
func (s *Service) RevokeAPIKey(ctx context.Context, actor *model.User, keyID string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	if uuid.Validate(keyID) != nil {
		return service.ErrNotFound
	}

	_, err := notFound(struct{}{}, s.store.DeleteAPIKey(ctx, actor.ID, keyID))

	return err
}

// UseAPIKey returns the API key whose plain value is key, with its user, and
// records the use in its LastUsedAt. An unknown or revoked key is not found.
func (s *Service) UseAPIKey(ctx context.Context, key string) (*model.UserAPIKey, error) {
	k, err := notFound(s.store.APIKeyByHash(ctx, model.HashAPIKey(key)))
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := s.store.TouchAPIKey(ctx, k.ID, now); err != nil {
		return nil, err
	}

	k.LastUsedAt = &now

	return k, nil
}
