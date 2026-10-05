package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// PostByID returns a post, draft or published.
func (s *Store) PostByID(ctx context.Context, id string) (*model.Post, error) {
	post, err := core.FindPost(ctx, s.exec, id)
	return toModel[model.Post](post), notFound(err)
}

// PostForEdit returns a post with its author, stats and linked URL loaded
// (post.User, post.PostStat, post.URL).
func (s *Store) PostForEdit(ctx context.Context, id string) (*model.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(id),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	).One(ctx, s.exec)

	return toModel[model.Post](post), notFound(err)
}

// OwnPostForUpdate locks and returns a post of the given author. With
// draftOnly a published post is not found.
func (s *Store) OwnPostForUpdate(ctx context.Context, postID, authorID string, draftOnly bool) (*model.Post, error) {
	mods := []qm.QueryMod{
		core.PostWhere.ID.EQ(postID),
		core.PostWhere.UserID.EQ(authorID),
		qm.For("UPDATE"),
	}

	if draftOnly {
		mods = append(mods, core.PostWhere.PublishedAt.IsNull())
	}

	post, err := core.Posts(mods...).One(ctx, s.exec)

	return toModel[model.Post](post), notFound(err)
}

// OwnPost returns a post of the given author, or ErrNotFound.
func (s *Store) OwnPost(ctx context.Context, postID, authorID string) (*model.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		core.PostWhere.UserID.EQ(authorID),
	).One(ctx, s.exec)

	return toModel[model.Post](post), notFound(err)
}

// OwnPostsByIDs returns those of the given posts that belong to the author.
func (s *Store) OwnPostsByIDs(ctx context.Context, authorID string, ids []string) ([]*model.Post, error) {
	return all[model.Post](core.Posts(
		core.PostWhere.UserID.EQ(authorID),
		core.PostWhere.ID.IN(ids),
	).All(ctx, s.exec))
}

// PostsForExport returns an author's posts with their linked URL loaded. With
// postID only that post is returned.
func (s *Store) PostsForExport(ctx context.Context, authorID, postID string) ([]*model.Post, error) {
	mods := []qm.QueryMod{
		core.PostWhere.UserID.EQ(authorID),
		qm.Load(core.PostRels.URL),
	}

	if postID != "" {
		mods = append(mods, core.PostWhere.ID.EQ(postID))
	}

	return all[model.Post](core.Posts(mods...).All(ctx, s.exec))
}

// PostsPage is the query the API lists posts with.
type PostsPage struct {
	AuthorID string
	// UpdatedSince keeps posts updated after it, when it is not zero.
	UpdatedSince time.Time
	// Cursor keeps posts with a smaller ID, when it is not empty.
	Cursor string
	Limit  int
}

// PostsByAuthor returns an author's posts, newest ID first.
func (s *Store) PostsByAuthor(ctx context.Context, page PostsPage) ([]*model.Post, error) {
	q := []qm.QueryMod{
		core.PostWhere.UserID.EQ(page.AuthorID),
		qm.OrderBy("id desc"),
		qm.Limit(page.Limit),
	}

	if !page.UpdatedSince.IsZero() {
		q = append(q, core.PostWhere.UpdatedAt.GT(null.TimeFrom(page.UpdatedSince)))
	}

	if page.Cursor != "" {
		q = append(q, core.PostWhere.ID.LT(page.Cursor))
	}

	return all[model.Post](core.Posts(q...).All(ctx, s.exec))
}

// InsertPost stores a new post.
func (s *Store) InsertPost(ctx context.Context, post *model.Post) error {
	return write(post, func(c *core.Post) error {
		return c.Insert(ctx, s.exec, boil.Infer())
	})
}

// UpdatePost writes every column of an existing post.
func (s *Store) UpdatePost(ctx context.Context, post *model.Post) error {
	return write(post, func(c *core.Post) error {
		_, err := c.Update(ctx, s.exec, boil.Infer())
		return err
	})
}

// DeletePost removes a post with everything that hangs off it: comments,
// stats, share link and the prompt it answered.
func (s *Store) DeletePost(ctx context.Context, postID string) error {
	if _, err := core.PostComments(core.PostCommentWhere.PostID.EQ(postID)).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	if _, err := core.PostStats(core.PostStatWhere.PostID.EQ(postID)).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	if _, err := core.PostShares(core.PostShareWhere.PostID.EQ(postID)).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	if _, err := core.PostPrompts(core.PostPromptWhere.PostID.EQ(null.StringFrom(postID))).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	_, err := core.Posts(core.PostWhere.ID.EQ(postID)).DeleteAll(ctx, s.exec)

	return err
}

// StoreURL normalizes the given URL and stores it in the normalized_urls
// table, keeping the existing row when the URL is known already.
func (s *Store) StoreURL(ctx context.Context, rawURL string) (*model.NormalizedURL, error) {
	normalizedURL, err := util.NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	newURL := &core.NormalizedURL{
		ID:  id.String(),
		URL: normalizedURL,
	}

	// true asks for the existing id back
	err = newURL.Upsert(ctx, s.exec, true, []string{core.NormalizedURLColumns.URL}, boil.Infer(), boil.Infer())
	if err != nil {
		return nil, err
	}

	return toModel[model.NormalizedURL](newURL), nil
}

// UploadsByName returns the user's uploads among the given file names.
func (s *Store) UploadsByName(ctx context.Context, userID string, names []string) ([]*model.MediaUpload, error) {
	return all[model.MediaUpload](core.MediaUploads(
		core.MediaUploadWhere.UploadedFname.IN(names),
		core.MediaUploadWhere.UserID.EQ(null.StringFrom(userID)),
	).All(ctx, s.exec))
}

// PostToRead returns a post, draft or published, with its author, stats and
// linked URL loaded (post.User, post.PostStat, post.URL).
func (s *Store) PostToRead(ctx context.Context, id string) (*model.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(id),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	).One(ctx, s.exec)

	return toModel[model.Post](post), notFound(err)
}

// postMods pages published posts by publication time.
func (p Page) postMods() []qm.QueryMod {
	return p.mods(KindPost, core.PostTableColumns.PublishedAt, core.PostTableColumns.ID)
}

// PublishedPostsOf returns a page of the author's published posts, newest
// first, with the author and linked URL loaded. visibilities limits the posts
// to those visibilities; nil means any. withStats loads post.PostStat too.
func (s *Store) PublishedPostsOf(ctx context.Context, authorID string, visibilities []model.PostVisibility, withStats bool, page Page) ([]*model.Post, error) {
	m := []qm.QueryMod{
		core.PostWhere.UserID.EQ(authorID),
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.URL),
	}

	if withStats {
		m = append(m, qm.Load(core.PostRels.PostStat))
	}

	if visibilities != nil {
		m = append(m, core.PostWhere.VisibilityRadius.IN(toCoreEnums[core.PostVisibility](visibilities)))
	}

	return all[model.Post](core.Posts(append(m, page.postMods()...)...).All(ctx, s.exec))
}

// PublishedPostsOfUsers returns a page, newest first, of the published posts
// of the users in allOf, and those of the users in someOf that have one of
// the visibilities given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsOfUsers(ctx context.Context, allOf, someOf []string, visibilities []model.PostVisibility, page Page) ([]*model.Post, error) {
	m := []qm.QueryMod{
		core.PostWhere.PublishedAt.IsNotNull(),
		qm.Expr(
			core.PostWhere.UserID.IN(allOf),
			qm.Or2(qm.Expr(
				core.PostWhere.UserID.IN(someOf),
				core.PostWhere.VisibilityRadius.IN(toCoreEnums[core.PostVisibility](visibilities)),
			))),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	}

	return all[model.Post](core.Posts(append(m, page.postMods()...)...).All(ctx, s.exec))
}

// PublishedPostsByProfile returns a page, newest first, of the published
// posts with the given visibility whose authors have one of the profile
// visibilities given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsByProfile(ctx context.Context, visibility model.PostVisibility, profiles []model.ProfileVisibility, page Page) ([]*model.Post, error) {
	m := []qm.QueryMod{
		core.PostWhere.PublishedAt.IsNotNull(),
		core.PostWhere.VisibilityRadius.EQ(core.PostVisibility(visibility)),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
		qm.LeftOuterJoin("users on users.ID = posts.user_id"),
		core.UserWhere.ProfileVisibility.IN(toCoreEnums[core.ProfileVisibility](profiles)),
	}

	return all[model.Post](core.Posts(append(m, page.postMods()...)...).All(ctx, s.exec))
}

// PostsOfOrAmong returns the posts, drafts included, written by userID, and
// those written by one of authorIDs whose ID is in postIDs, with their
// authors loaded.
func (s *Store) PostsOfOrAmong(ctx context.Context, userID string, authorIDs, postIDs []string) ([]*model.Post, error) {
	return all[model.Post](core.Posts(
		qm.Expr(
			core.PostWhere.UserID.EQ(userID),
			qm.Or2(
				qm.Expr(
					core.PostWhere.UserID.IN(authorIDs),
					core.PostWhere.ID.IN(postIDs),
				))),
		qm.Load(core.PostRels.User),
	).All(ctx, s.exec))
}

// DraftPosts returns the unpublished posts of a user, the last edited first.
func (s *Store) DraftPosts(ctx context.Context, userID string) ([]*model.Post, error) {
	return all[model.Post](core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.PublishedAt.IsNull(),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.UpdatedAt)),
	).All(ctx, s.exec))
}
