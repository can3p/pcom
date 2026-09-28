package factory

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// GetUser looks up a user by id.
func GetUser(ctx context.Context, exec boil.ContextExecutor, id string) (*core.User, error) {
	return core.FindUser(ctx, exec, id)
}

// GetUserByEmail looks up a user by email, for tests of code that creates one.
func GetUserByEmail(ctx context.Context, exec boil.ContextExecutor, email string) (*core.User, error) {
	return core.Users(core.UserWhere.Email.EQ(email)).One(ctx, exec)
}

// GetUserStyle returns userID's custom styles, or sql.ErrNoRows if there are none.
func GetUserStyle(ctx context.Context, exec boil.ContextExecutor, userID string) (*core.UserStyle, error) {
	return core.UserStyles(core.UserStyleWhere.UserID.EQ(userID)).One(ctx, exec)
}

// GetRSSItem looks up a feed item by id.
func GetRSSItem(ctx context.Context, exec boil.ContextExecutor, id string) (*core.RSSItem, error) {
	return core.FindRSSItem(ctx, exec, id)
}

// ListRSSItems returns every item stored for feedID.
func ListRSSItems(ctx context.Context, exec boil.ContextExecutor, feedID string) (core.RSSItemSlice, error) {
	return core.RSSItems(core.RSSItemWhere.FeedID.EQ(feedID)).All(ctx, exec)
}

// GetPost looks up a post by id.
func GetPost(ctx context.Context, exec boil.ContextExecutor, id string) (*core.Post, error) {
	return core.FindPost(ctx, exec, id)
}

// ListPosts returns every post owned by userID.
func ListPosts(ctx context.Context, exec boil.ContextExecutor, userID string) (core.PostSlice, error) {
	return core.Posts(core.PostWhere.UserID.EQ(userID)).All(ctx, exec)
}

// ListComments returns every comment on postID.
func ListComments(ctx context.Context, exec boil.ContextExecutor, postID string) (core.PostCommentSlice, error) {
	return core.PostComments(core.PostCommentWhere.PostID.EQ(postID)).All(ctx, exec)
}

// GetPostStat returns the stat row of postID, or sql.ErrNoRows if none exists yet.
func GetPostStat(ctx context.Context, exec boil.ContextExecutor, postID string) (*core.PostStat, error) {
	return core.PostStats(core.PostStatWhere.PostID.EQ(postID)).One(ctx, exec)
}

// GetMediaUploadByFname looks up the upload row that HandleUpload created for fname.
func GetMediaUploadByFname(ctx context.Context, exec boil.ContextExecutor, fname string) (*core.MediaUpload, error) {
	return core.MediaUploads(core.MediaUploadWhere.UploadedFname.EQ(fname)).One(ctx, exec)
}

// ListOutgoingEmails returns the queued emails matching filter, e.g.
// factory.ListOutgoingEmails(ctx, db, core.OutgoingEmailWhere.EmailType.EQ("welcome")).
func ListOutgoingEmails(ctx context.Context, exec boil.ContextExecutor, filter ...qm.QueryMod) (core.OutgoingEmailSlice, error) {
	return core.OutgoingEmails(filter...).All(ctx, exec)
}

// ConnectionExists reports whether aID and bID are directly connected, in
// either direction.
func ConnectionExists(ctx context.Context, exec boil.ContextExecutor, aID, bID string) (bool, error) {
	return core.UserConnections(
		qm.Expr(
			core.UserConnectionWhere.User1ID.EQ(aID),
			core.UserConnectionWhere.User2ID.EQ(bID),
		),
		qm.Or2(qm.Expr(
			core.UserConnectionWhere.User1ID.EQ(bID),
			core.UserConnectionWhere.User2ID.EQ(aID),
		)),
	).Exists(ctx, exec)
}

// GetMediationRequest looks up a mediation request by id.
func GetMediationRequest(ctx context.Context, exec boil.ContextExecutor, id string) (*core.UserConnectionMediationRequest, error) {
	return core.FindUserConnectionMediationRequest(ctx, exec, id)
}

// GetSignupRequest looks up a waiting-list request by id.
func GetSignupRequest(ctx context.Context, exec boil.ContextExecutor, id string) (*core.UserSignupRequest, error) {
	return core.FindUserSignupRequest(ctx, exec, id)
}

// ListMediatorDecisions returns the mediators' decisions on mediation request requestID.
func ListMediatorDecisions(ctx context.Context, exec boil.ContextExecutor, requestID string) (core.UserConnectionMediatorSlice, error) {
	return core.UserConnectionMediators(core.UserConnectionMediatorWhere.MediationID.EQ(requestID)).All(ctx, exec)
}

// GetPostShare looks up a post share by id.
func GetPostShare(ctx context.Context, exec boil.ContextExecutor, id string) (*core.PostShare, error) {
	return core.FindPostShare(ctx, exec, id)
}

// SubscriptionExists reports whether userID subscribes to feedID.
func SubscriptionExists(ctx context.Context, exec boil.ContextExecutor, userID, feedID string) (bool, error) {
	return core.UserFeedSubscriptions(
		core.UserFeedSubscriptionWhere.UserID.EQ(userID),
		core.UserFeedSubscriptionWhere.FeedID.EQ(feedID),
	).Exists(ctx, exec)
}

// GetUserFeedItem looks up a user's feed item by id.
func GetUserFeedItem(ctx context.Context, exec boil.ContextExecutor, id string) (*core.UserFeedItem, error) {
	return core.FindUserFeedItem(ctx, exec, id)
}

// GetPostPrompt looks up a post prompt by id.
func GetPostPrompt(ctx context.Context, exec boil.ContextExecutor, id string) (*core.PostPrompt, error) {
	return core.FindPostPrompt(ctx, exec, id)
}

// WhitelistExists reports whether whoID allows allowsWhoID to connect.
func WhitelistExists(ctx context.Context, exec boil.ContextExecutor, whoID, allowsWhoID string) (bool, error) {
	return core.WhitelistedConnections(
		core.WhitelistedConnectionWhere.WhoID.EQ(whoID),
		core.WhitelistedConnectionWhere.AllowsWhoID.EQ(allowsWhoID),
	).Exists(ctx, exec)
}

// ShareExists reports whether postID has a share link.
func ShareExists(ctx context.Context, exec boil.ContextExecutor, postID string) (bool, error) {
	return core.PostShares(core.PostShareWhere.PostID.EQ(postID)).Exists(ctx, exec)
}

// SignupRequestExists reports whether a waiting-list request exists for email.
func SignupRequestExists(ctx context.Context, exec boil.ContextExecutor, email string) (bool, error) {
	return core.UserSignupRequests(core.UserSignupRequestWhere.Email.EQ(email)).Exists(ctx, exec)
}

// ListAPIKeys returns userID's API keys.
func ListAPIKeys(ctx context.Context, exec boil.ContextExecutor, userID string) (core.UserAPIKeySlice, error) {
	return core.UserAPIKeys(core.UserAPIKeyWhere.UserID.EQ(userID)).All(ctx, exec)
}
