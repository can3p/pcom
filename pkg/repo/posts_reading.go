package repo

import (
	"context"
	"fmt"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// PostToRead returns a post, draft or published, with its author, stats and
// linked URL loaded (post.R.User, post.R.PostStat, post.R.URL).
func (s *Store) PostToRead(ctx context.Context, id string) (*core.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(id),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	).One(ctx, s.exec)

	return post, notFound(err)
}

// PublishedPostsOf returns the author's published posts, newest first, with
// the author and linked URL loaded. visibilities limits the posts to those
// visibilities; nil means any. withStats loads post.R.PostStat too.
func (s *Store) PublishedPostsOf(ctx context.Context, authorID string, visibilities []core.PostVisibility, withStats bool) (core.PostSlice, error) {
	m := []qm.QueryMod{
		core.PostWhere.UserID.EQ(authorID),
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.URL),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	}

	if withStats {
		m = append(m, qm.Load(core.PostRels.PostStat))
	}

	if visibilities != nil {
		m = append(m, core.PostWhere.VisibilityRadius.IN(visibilities))
	}

	return core.Posts(m...).All(ctx, s.exec)
}

// PublishedPostsOfUsers returns, newest first, the published posts of the
// users in allOf, and those of the users in someOf that have one of the
// visibilities given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsOfUsers(ctx context.Context, allOf, someOf []string, visibilities []core.PostVisibility) (core.PostSlice, error) {
	return core.Posts(
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Expr(
			core.PostWhere.UserID.IN(allOf),
			qm.Or2(qm.Expr(
				core.PostWhere.UserID.IN(someOf),
				core.PostWhere.VisibilityRadius.IN(visibilities),
			))),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, s.exec)
}

// PublishedPostsByProfile returns, newest first, the published posts with
// the given visibility whose authors have one of the profile visibilities
// given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsByProfile(ctx context.Context, visibility core.PostVisibility, profiles []core.ProfileVisibility) (core.PostSlice, error) {
	return core.Posts(
		core.PostWhere.PublishedAt.IsNotNull(),
		core.PostWhere.VisibilityRadius.EQ(visibility),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
		qm.LeftOuterJoin("users on users.ID = posts.user_id"),
		core.UserWhere.ProfileVisibility.IN(profiles),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	).All(ctx, s.exec)
}

// PostsOfOrAmong returns the posts, drafts included, written by userID, and
// those written by one of authorIDs whose ID is in postIDs, with their
// authors loaded.
func (s *Store) PostsOfOrAmong(ctx context.Context, userID string, authorIDs, postIDs []string) (core.PostSlice, error) {
	return core.Posts(
		qm.Expr(
			core.PostWhere.UserID.EQ(userID),
			qm.Or2(
				qm.Expr(
					core.PostWhere.UserID.IN(authorIDs),
					core.PostWhere.ID.IN(postIDs),
				))),
		qm.Load(core.PostRels.User),
	).All(ctx, s.exec)
}
