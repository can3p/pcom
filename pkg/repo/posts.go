package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// PostByID returns a post, draft or published.
func (s *Store) PostByID(ctx context.Context, id string) (*core.Post, error) {
	post, err := core.FindPost(ctx, s.exec, id)
	return post, notFound(err)
}

// PostForEdit returns a post with its author, stats and linked URL loaded
// (post.R.User, post.R.PostStat, post.R.URL).
func (s *Store) PostForEdit(ctx context.Context, id string) (*core.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(id),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
	).One(ctx, s.exec)

	return post, notFound(err)
}

// OwnPostForUpdate locks and returns a post of the given author. With
// draftOnly a published post is not found.
func (s *Store) OwnPostForUpdate(ctx context.Context, postID, authorID string, draftOnly bool) (*core.Post, error) {
	mods := []qm.QueryMod{
		core.PostWhere.ID.EQ(postID),
		core.PostWhere.UserID.EQ(authorID),
		qm.For("UPDATE"),
	}

	if draftOnly {
		mods = append(mods, core.PostWhere.PublishedAt.IsNull())
	}

	post, err := core.Posts(mods...).One(ctx, s.exec)

	return post, notFound(err)
}

// OwnPost returns a post of the given author, or ErrNotFound.
func (s *Store) OwnPost(ctx context.Context, postID, authorID string) (*core.Post, error) {
	post, err := core.Posts(
		core.PostWhere.ID.EQ(postID),
		core.PostWhere.UserID.EQ(authorID),
	).One(ctx, s.exec)

	return post, notFound(err)
}

// OwnPostsByIDs returns those of the given posts that belong to the author.
func (s *Store) OwnPostsByIDs(ctx context.Context, authorID string, ids []string) ([]*core.Post, error) {
	return core.Posts(
		core.PostWhere.UserID.EQ(authorID),
		core.PostWhere.ID.IN(ids),
	).All(ctx, s.exec)
}

// PostsForExport returns an author's posts with their linked URL loaded. With
// postID only that post is returned.
func (s *Store) PostsForExport(ctx context.Context, authorID, postID string) ([]*core.Post, error) {
	mods := []qm.QueryMod{
		core.PostWhere.UserID.EQ(authorID),
		qm.Load(core.PostRels.URL),
	}

	if postID != "" {
		mods = append(mods, core.PostWhere.ID.EQ(postID))
	}

	return core.Posts(mods...).All(ctx, s.exec)
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
func (s *Store) PostsByAuthor(ctx context.Context, page PostsPage) ([]*core.Post, error) {
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

	return core.Posts(q...).All(ctx, s.exec)
}

// InsertPost stores a new post.
func (s *Store) InsertPost(ctx context.Context, post *core.Post) error {
	return post.Insert(ctx, s.exec, boil.Infer())
}

// UpdatePost writes every column of an existing post.
func (s *Store) UpdatePost(ctx context.Context, post *core.Post) error {
	_, err := post.Update(ctx, s.exec, boil.Infer())
	return err
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
func (s *Store) StoreURL(ctx context.Context, rawURL string) (*core.NormalizedURL, error) {
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

	return newURL, nil
}

// UploadsByName returns the user's uploads among the given file names.
func (s *Store) UploadsByName(ctx context.Context, userID string, names []string) ([]*core.MediaUpload, error) {
	return core.MediaUploads(
		core.MediaUploadWhere.UploadedFname.IN(names),
		core.MediaUploadWhere.UserID.EQ(null.StringFrom(userID)),
	).All(ctx, s.exec)
}

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
// given, at most limit of them (0 means no limit). Author, stats and linked
// URL are loaded.
func (s *Store) PublishedPostsByProfile(ctx context.Context, visibility core.PostVisibility, profiles []core.ProfileVisibility, limit int) (core.PostSlice, error) {
	mods := []qm.QueryMod{
		core.PostWhere.PublishedAt.IsNotNull(),
		core.PostWhere.VisibilityRadius.EQ(visibility),
		qm.Load(core.PostRels.User),
		qm.Load(core.PostRels.PostStat),
		qm.Load(core.PostRels.URL),
		qm.LeftOuterJoin("users on users.ID = posts.user_id"),
		core.UserWhere.ProfileVisibility.IN(profiles),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.PublishedAt)),
	}

	if limit > 0 {
		mods = append(mods, qm.Limit(limit))
	}

	return core.Posts(mods...).All(ctx, s.exec)
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

// DraftPosts returns the unpublished posts of a user, the last edited first.
func (s *Store) DraftPosts(ctx context.Context, userID string) ([]*core.Post, error) {
	return core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.PublishedAt.IsNull(),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.UpdatedAt)),
	).All(ctx, s.exec)
}
