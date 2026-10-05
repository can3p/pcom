// Package repo holds database queries.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// The free functions predate Store; e2e/ calls them, so they stay until RS
// may edit e2e/.

func FeedTokenOwner(ctx context.Context, exec boil.ContextExecutor, token string) (*model.User, error) {
	u, err := Using(exec).FeedTokenOwner(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return nil, sql.ErrNoRows
	}

	return u, err
}

func FeedTokenForUser(ctx context.Context, exec boil.ContextExecutor, userID string) (*model.UserFeedToken, error) {
	return Using(exec).FeedTokenForUser(ctx, userID)
}

func RegenerateFeedToken(ctx context.Context, exec boil.ContextExecutor, userID string) (*model.UserFeedToken, error) {
	return Using(exec).RegenerateFeedToken(ctx, userID)
}

// FeedTokenOwner returns the user a private RSS feed token belongs to, or
// ErrNotFound when the token is unknown.
func (s *Store) FeedTokenOwner(ctx context.Context, token string) (*model.User, error) {
	t, err := core.UserFeedTokens(
		core.UserFeedTokenWhere.Token.EQ(token),
		qm.Load(core.UserFeedTokenRels.User),
	).One(ctx, s.exec)
	if err != nil {
		return nil, notFound(err)
	}

	return toModel[model.User](t.R.User), nil
}

// FeedTokenForUser returns the user's feed token, or nil when they have none.
func (s *Store) FeedTokenForUser(ctx context.Context, userID string) (*model.UserFeedToken, error) {
	t, err := core.UserFeedTokens(core.UserFeedTokenWhere.UserID.EQ(userID)).One(ctx, s.exec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return toModel[model.UserFeedToken](t), err
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

	record := &core.UserFeedToken{ID: id.String(), UserID: userID, Token: token.String()}

	err = record.Upsert(ctx, s.exec, true, []string{core.UserFeedTokenColumns.UserID},
		boil.Whitelist(core.UserFeedTokenColumns.Token, core.UserFeedTokenColumns.UpdatedAt), boil.Infer())
	if err != nil {
		return nil, err
	}

	return toModel[model.UserFeedToken](record), nil
}
