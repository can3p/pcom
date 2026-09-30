package repo

import (
	"context"
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

// UsersByIDsForMail returns the users with the given IDs.
func (s *Store) UsersByIDsForMail(ctx context.Context, ids []string) ([]*core.User, error) {
	return core.Users(core.UserWhere.ID.IN(ids)).All(ctx, s.exec)
}

// UploadsByName returns the user's uploads among the given file names.
func (s *Store) UploadsByName(ctx context.Context, userID string, names []string) ([]*core.MediaUpload, error) {
	return core.MediaUploads(
		core.MediaUploadWhere.UploadedFname.IN(names),
		core.MediaUploadWhere.UserID.EQ(null.StringFrom(userID)),
	).All(ctx, s.exec)
}

// InsertUpload records an uploaded file.
func (s *Store) InsertUpload(ctx context.Context, upload *core.MediaUpload) error {
	return upload.Insert(ctx, s.exec, boil.Infer())
}
