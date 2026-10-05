// Package repo holds database queries.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

// The free functions predate Store; e2e/ calls them, so they stay until RS
// may edit e2e/.

func FeedTokenOwner(ctx context.Context, exec Executor, token string) (*model.User, error) {
	u, err := Using(exec).FeedTokenOwner(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return nil, sql.ErrNoRows
	}

	return u, err
}

func FeedTokenForUser(ctx context.Context, exec Executor, userID string) (*model.UserFeedToken, error) {
	return Using(exec).FeedTokenForUser(ctx, userID)
}

func RegenerateFeedToken(ctx context.Context, exec Executor, userID string) (*model.UserFeedToken, error) {
	return Using(exec).RegenerateFeedToken(ctx, userID)
}

// FeedTokenOwner returns the user a private RSS feed token belongs to, or
// ErrNotFound when the token is unknown.
func (s *Store) FeedTokenOwner(ctx context.Context, token string) (*model.User, error) {
	t := new(model.UserFeedToken)

	err := s.query().NewSelect().Model(t).
		Relation("User").
		Where("?TableAlias.token = ?", token).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return t.User, nil
}

// FeedTokenForUser returns the user's feed token, or nil when they have none.
func (s *Store) FeedTokenForUser(ctx context.Context, userID string) (*model.UserFeedToken, error) {
	t := new(model.UserFeedToken)

	err := s.query().NewSelect().Model(t).
		Where("user_id = ?", userID).
		Limit(1).
		Scan(ctx)

	return orNil(t, err)
}

// RegenerateFeedToken creates the user's feed token or replaces the existing
// one, which stops working at once.
func (s *Store) RegenerateFeedToken(ctx context.Context, userID string) (*model.UserFeedToken, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	token, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	record := &model.UserFeedToken{ID: id.String(), UserID: userID, Token: token.String(), UpdatedAt: time.Now()}

	_, err = s.query().NewInsert().Model(record).
		On("CONFLICT (user_id) DO UPDATE").
		Set("token = EXCLUDED.token").
		Set("updated_at = EXCLUDED.updated_at").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return record, nil
}
