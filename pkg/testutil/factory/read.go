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
