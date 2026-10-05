package factory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/uptrace/bun"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// noRows maps the repository's ErrNotFound back to sql.ErrNoRows, which the
// helpers here have always returned.
func noRows[T any](v *T, err error) (*T, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return nil, sql.ErrNoRows
	}

	return v, err
}

// GetUser looks up a user by id.
func GetUser(ctx context.Context, exec boil.ContextExecutor, id string) (*model.User, error) {
	return noRows(repo.Using(exec).UserByID(ctx, id))
}

// GetUserByEmail looks up a user by email, for tests of code that creates one.
func GetUserByEmail(ctx context.Context, exec boil.ContextExecutor, email string) (*model.User, error) {
	return noRows(repo.Using(exec).UserByEmail(ctx, email))
}

// GetUserStyle returns userID's custom styles, or sql.ErrNoRows if there are none.
func GetUserStyle(ctx context.Context, exec boil.ContextExecutor, userID string) (*model.UserStyle, error) {
	return one[model.UserStyle](ctx, exec, "user_id = ?", userID)
}

// GetRSSItem looks up a feed item by id.
func GetRSSItem(ctx context.Context, exec boil.ContextExecutor, id string) (*model.RSSItem, error) {
	return find(ctx, exec, &model.RSSItem{ID: id})
}

// ListRSSItems returns every item stored for feedID.
func ListRSSItems(ctx context.Context, exec boil.ContextExecutor, feedID string) ([]*model.RSSItem, error) {
	return list[model.RSSItem](ctx, exec, "feed_id = ?", feedID)
}

// GetPost looks up a post by id.
func GetPost(ctx context.Context, exec boil.ContextExecutor, id string) (*model.Post, error) {
	return find(ctx, exec, &model.Post{ID: id})
}

// ListPosts returns every post owned by userID.
func ListPosts(ctx context.Context, exec boil.ContextExecutor, userID string) ([]*model.Post, error) {
	return list[model.Post](ctx, exec, "user_id = ?", userID)
}

// ListComments returns every comment on postID.
func ListComments(ctx context.Context, exec boil.ContextExecutor, postID string) ([]*model.PostComment, error) {
	return list[model.PostComment](ctx, exec, "post_id = ?", postID)
}

// GetPostStat returns the stat row of postID, or sql.ErrNoRows if none exists yet.
func GetPostStat(ctx context.Context, exec boil.ContextExecutor, postID string) (*model.PostStat, error) {
	return one[model.PostStat](ctx, exec, "post_id = ?", postID)
}

// GetMediaUploadByFname looks up the upload row that StoreUpload created for fname.
func GetMediaUploadByFname(ctx context.Context, exec boil.ContextExecutor, fname string) (*model.MediaUpload, error) {
	return one[model.MediaUpload](ctx, exec, "uploaded_fname = ?", fname)
}

// OutgoingEmailFilter narrows ListOutgoingEmails.
type OutgoingEmailFilter func(*bun.SelectQuery) *bun.SelectQuery

// EmailType keeps the emails of type t.
func EmailType(t string) OutgoingEmailFilter {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("email_type = ?", t)
	}
}

// ListOutgoingEmails returns the queued emails matching every filter, e.g.
// factory.ListOutgoingEmails(ctx, db, factory.EmailType("welcome")).
func ListOutgoingEmails(ctx context.Context, exec boil.ContextExecutor, filter ...OutgoingEmailFilter) ([]*model.OutgoingEmail, error) {
	var emails []*model.OutgoingEmail

	q := repo.Query(exec).NewSelect().Model(&emails)
	for _, f := range filter {
		q = f(q)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, err
	}

	return emails, nil
}

// ConnectionExists reports whether aID and bID are directly connected, in
// either direction.
func ConnectionExists(ctx context.Context, exec boil.ContextExecutor, aID, bID string) (bool, error) {
	return exists[model.UserConnection](ctx, exec,
		"(user1_id = ? AND user2_id = ?) OR (user1_id = ? AND user2_id = ?)", aID, bID, bID, aID)
}

// GetMediationRequest looks up a mediation request by id.
func GetMediationRequest(ctx context.Context, exec boil.ContextExecutor, id string) (*model.UserConnectionMediationRequest, error) {
	return find(ctx, exec, &model.UserConnectionMediationRequest{ID: id})
}

// GetSignupRequest looks up a waiting-list request by id.
func GetSignupRequest(ctx context.Context, exec boil.ContextExecutor, id string) (*model.UserSignupRequest, error) {
	return noRows(repo.Using(exec).SignupRequestByID(ctx, id))
}

// ListMediatorDecisions returns the mediators' decisions on mediation request requestID.
func ListMediatorDecisions(ctx context.Context, exec boil.ContextExecutor, requestID string) ([]*model.UserConnectionMediator, error) {
	return list[model.UserConnectionMediator](ctx, exec, "mediation_id = ?", requestID)
}

// GetPostShare looks up a post share by id.
func GetPostShare(ctx context.Context, exec boil.ContextExecutor, id string) (*model.PostShare, error) {
	return noRows(repo.Using(exec).ShareByID(ctx, id))
}

// SubscriptionExists reports whether userID subscribes to feedID.
func SubscriptionExists(ctx context.Context, exec boil.ContextExecutor, userID, feedID string) (bool, error) {
	return exists[model.UserFeedSubscription](ctx, exec, "user_id = ? AND feed_id = ?", userID, feedID)
}

// GetUserFeedItem looks up a user's feed item by id.
func GetUserFeedItem(ctx context.Context, exec boil.ContextExecutor, id string) (*model.UserFeedItem, error) {
	return find(ctx, exec, &model.UserFeedItem{ID: id})
}

// GetPostPrompt looks up a post prompt by id.
func GetPostPrompt(ctx context.Context, exec boil.ContextExecutor, id string) (*model.PostPrompt, error) {
	return find(ctx, exec, &model.PostPrompt{ID: id})
}

// WhitelistExists reports whether whoID allows allowsWhoID to connect.
func WhitelistExists(ctx context.Context, exec boil.ContextExecutor, whoID, allowsWhoID string) (bool, error) {
	return repo.Using(exec).GrantExists(ctx, whoID, allowsWhoID)
}

// ShareExists reports whether postID has a share link.
func ShareExists(ctx context.Context, exec boil.ContextExecutor, postID string) (bool, error) {
	return exists[model.PostShare](ctx, exec, "post_id = ?", postID)
}

// SignupRequestExists reports whether a waiting-list request exists for email.
func SignupRequestExists(ctx context.Context, exec boil.ContextExecutor, email string) (bool, error) {
	return repo.Using(exec).SignupRequestEmailExists(ctx, email)
}

// ListAPIKeys returns userID's API keys.
func ListAPIKeys(ctx context.Context, exec boil.ContextExecutor, userID string) ([]*model.UserAPIKey, error) {
	return list[model.UserAPIKey](ctx, exec, "user_id = ?", userID)
}
