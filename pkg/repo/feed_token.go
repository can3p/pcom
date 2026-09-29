// Package repo holds database queries.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// FeedTokenOwner returns the user a private RSS feed token belongs to, or
// sql.ErrNoRows when the token is unknown.
func FeedTokenOwner(ctx context.Context, exec boil.ContextExecutor, token string) (*core.User, error) {
	t, err := core.UserFeedTokens(
		core.UserFeedTokenWhere.Token.EQ(token),
		qm.Load(core.UserFeedTokenRels.User),
	).One(ctx, exec)
	if err != nil {
		return nil, err
	}

	return t.R.User, nil
}

// FeedTokenForUser returns the user's feed token, or nil when they have none.
func FeedTokenForUser(ctx context.Context, exec boil.ContextExecutor, userID string) (*core.UserFeedToken, error) {
	t, err := core.UserFeedTokens(core.UserFeedTokenWhere.UserID.EQ(userID)).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return t, err
}

// RegenerateFeedToken creates the user's feed token or replaces the existing
// one, which stops working at once.
func RegenerateFeedToken(ctx context.Context, exec boil.ContextExecutor, userID string) (*core.UserFeedToken, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	token, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	record := &core.UserFeedToken{ID: id.String(), UserID: userID, Token: token.String()}

	err = record.Upsert(ctx, exec, true, []string{core.UserFeedTokenColumns.UserID},
		boil.Whitelist(core.UserFeedTokenColumns.Token, core.UserFeedTokenColumns.UpdatedAt), boil.Infer())
	if err != nil {
		return nil, err
	}

	return record, nil
}
