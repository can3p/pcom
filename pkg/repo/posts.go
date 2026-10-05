package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/util"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// PostByID returns a post, draft or published.
func (s *Store) PostByID(ctx context.Context, id string) (*model.Post, error) {
	post := new(model.Post)
	err := s.query().NewSelect().Model(post).Where("id = ?", id).Scan(ctx)

	return post, notFound(err)
}

// PostForEdit returns a post with its author, stats and linked URL loaded
// (post.User, post.PostStat, post.URL).
func (s *Store) PostForEdit(ctx context.Context, id string) (*model.Post, error) {
	return s.postWithRelations(ctx, id)
}

// postWithRelations returns a post with its author, stats and linked URL.
func (s *Store) postWithRelations(ctx context.Context, id string) (*model.Post, error) {
	post := new(model.Post)
	err := s.query().NewSelect().Model(post).
		Relation("User").
		Relation("PostStat").
		Relation("URL").
		Where("?TableAlias.id = ?", id).
		Limit(1).
		Scan(ctx)

	return post, notFound(err)
}

// OwnPostForUpdate locks and returns a post of the given author. With
// draftOnly a published post is not found.
func (s *Store) OwnPostForUpdate(ctx context.Context, postID, authorID string, draftOnly bool) (*model.Post, error) {
	post := new(model.Post)
	q := s.query().NewSelect().Model(post).
		Where("id = ?", postID).
		Where("user_id = ?", authorID).
		For("UPDATE")

	if draftOnly {
		q = q.Where("published_at IS NULL")
	}

	err := q.Limit(1).Scan(ctx)

	return post, notFound(err)
}

// OwnPost returns a post of the given author, or ErrNotFound.
func (s *Store) OwnPost(ctx context.Context, postID, authorID string) (*model.Post, error) {
	post := new(model.Post)
	err := s.query().NewSelect().Model(post).
		Where("id = ?", postID).
		Where("user_id = ?", authorID).
		Limit(1).
		Scan(ctx)

	return post, notFound(err)
}

// OwnPostsByIDs returns those of the given posts that belong to the author.
func (s *Store) OwnPostsByIDs(ctx context.Context, authorID string, ids []string) ([]*model.Post, error) {
	var posts []*model.Post
	err := s.query().NewSelect().Model(&posts).
		Where("user_id = ?", authorID).
		Where("id IN (?)", bun.List(ids)).
		Scan(ctx)

	return posts, err
}

// PostsForExport returns an author's posts with their linked URL loaded. With
// postID only that post is returned.
func (s *Store) PostsForExport(ctx context.Context, authorID, postID string) ([]*model.Post, error) {
	var posts []*model.Post
	q := s.query().NewSelect().Model(&posts).
		Relation("URL").
		Where("?TableAlias.user_id = ?", authorID)

	if postID != "" {
		q = q.Where("?TableAlias.id = ?", postID)
	}

	err := q.Scan(ctx)

	return posts, err
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
	// LIMIT 0 matches no row, and bun would drop it
	if page.Limit == 0 {
		return nil, nil
	}

	var posts []*model.Post
	q := s.query().NewSelect().Model(&posts).
		Where("user_id = ?", page.AuthorID).
		OrderExpr("id desc").
		Limit(int64(page.Limit))

	if !page.UpdatedSince.IsZero() {
		q = q.Where("updated_at > ?", page.UpdatedSince)
	}

	if page.Cursor != "" {
		q = q.Where("id < ?", page.Cursor)
	}

	err := q.Scan(ctx)

	return posts, err
}

// InsertPost stores a new post.
func (s *Store) InsertPost(ctx context.Context, post *model.Post) error {
	_, err := s.query().NewInsert().Model(post).Exec(ctx)
	return err
}

// UpdatePost writes every column of an existing post.
func (s *Store) UpdatePost(ctx context.Context, post *model.Post) error {
	_, err := s.query().NewUpdate().Model(post).WherePK().Exec(ctx)
	return err
}

// DeletePost removes a post with everything that hangs off it: comments,
// stats, share link and the prompt it answered.
func (s *Store) DeletePost(ctx context.Context, postID string) error {
	q := s.query()

	if _, err := q.NewDelete().Model((*model.PostComment)(nil)).Where("post_id = ?", postID).Exec(ctx); err != nil {
		return err
	}

	if _, err := q.NewDelete().Model((*model.PostStat)(nil)).Where("post_id = ?", postID).Exec(ctx); err != nil {
		return err
	}

	if _, err := q.NewDelete().Model((*model.PostShare)(nil)).Where("post_id = ?", postID).Exec(ctx); err != nil {
		return err
	}

	if _, err := q.NewDelete().Model((*model.PostPrompt)(nil)).Where("post_id = ?", postID).Exec(ctx); err != nil {
		return err
	}

	_, err := q.NewDelete().Model((*model.Post)(nil)).Where("id = ?", postID).Exec(ctx)

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

	now := time.Now().UTC()
	newURL := &model.NormalizedURL{
		ID:        id.String(),
		URL:       normalizedURL,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// On a known URL the update makes Postgres return the existing row's id.
	// It sets every column but the key, as the upsert this replaces did.
	_, err = s.query().NewInsert().Model(newURL).
		On("CONFLICT (url) DO UPDATE").
		Set("url = EXCLUDED.url").
		Set("created_at = EXCLUDED.created_at").
		Set("updated_at = EXCLUDED.updated_at").
		Returning("id").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return newURL, nil
}

// UploadsByName returns the user's uploads among the given file names.
func (s *Store) UploadsByName(ctx context.Context, userID string, names []string) ([]*model.MediaUpload, error) {
	var uploads []*model.MediaUpload
	err := s.query().NewSelect().Model(&uploads).
		Where("uploaded_fname IN (?)", bun.List(names)).
		Where("user_id = ?", userID).
		Scan(ctx)

	return uploads, err
}

// PostToRead returns a post, draft or published, with its author, stats and
// linked URL loaded (post.User, post.PostStat, post.URL).
func (s *Store) PostToRead(ctx context.Context, id string) (*model.Post, error) {
	return s.postWithRelations(ctx, id)
}

// publishedPosts starts a list of published posts into posts, with the
// author and linked URL loaded.
func (s *Store) publishedPosts(posts *[]*model.Post) *bun.SelectQuery {
	return s.query().NewSelect().Model(posts).
		Relation("User").
		Relation("URL").
		Where("?TableAlias.published_at IS NOT NULL")
}

// pagePosts sorts a list of posts newest first and keeps the page.
func pagePosts(q *bun.SelectQuery, page Page) *bun.SelectQuery {
	return page.apply(q, KindPost, "?TableAlias.published_at", "?TableAlias.id")
}

// PublishedPostsOf returns a page of the author's published posts, newest
// first, with the author and linked URL loaded. visibilities limits the posts
// to those visibilities; nil means any. withStats loads post.PostStat too.
func (s *Store) PublishedPostsOf(ctx context.Context, authorID string, visibilities []model.PostVisibility, withStats bool, page Page) ([]*model.Post, error) {
	var posts []*model.Post
	q := s.publishedPosts(&posts).Where("?TableAlias.user_id = ?", authorID)

	if withStats {
		q = q.Relation("PostStat")
	}

	if visibilities != nil {
		q = q.Where("?TableAlias.visibility_radius IN (?)", bun.List(visibilities))
	}

	err := pagePosts(q, page).Scan(ctx)

	return posts, err
}

// PublishedPostsOfUsers returns a page, newest first, of the published posts
// of the users in allOf, and those of the users in someOf that have one of
// the visibilities given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsOfUsers(ctx context.Context, allOf, someOf []string, visibilities []model.PostVisibility, page Page) ([]*model.Post, error) {
	var posts []*model.Post
	// published AND (user_id IN allOf OR (user_id IN someOf AND visibility IN visibilities))
	q := s.publishedPosts(&posts).
		Relation("PostStat").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("?TableAlias.user_id IN (?)", bun.List(allOf)).
				WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return q.
						Where("?TableAlias.user_id IN (?)", bun.List(someOf)).
						Where("?TableAlias.visibility_radius IN (?)", bun.List(visibilities))
				})
		})

	err := pagePosts(q, page).Scan(ctx)

	return posts, err
}

// PublishedPostsByProfile returns a page, newest first, of the published
// posts with the given visibility whose authors have one of the profile
// visibilities given. Author, stats and linked URL are loaded.
func (s *Store) PublishedPostsByProfile(ctx context.Context, visibility model.PostVisibility, profiles []model.ProfileVisibility, page Page) ([]*model.Post, error) {
	var posts []*model.Post
	// "user" is the alias bun joins the User relation under
	q := s.publishedPosts(&posts).
		Relation("PostStat").
		Where("?TableAlias.visibility_radius = ?", visibility).
		Where(`"user".profile_visibility IN (?)`, bun.List(profiles))

	err := pagePosts(q, page).Scan(ctx)

	return posts, err
}

// PostsOfOrAmong returns the posts, drafts included, written by userID, and
// those written by one of authorIDs whose ID is in postIDs, with their
// authors loaded.
func (s *Store) PostsOfOrAmong(ctx context.Context, userID string, authorIDs, postIDs []string) ([]*model.Post, error) {
	var posts []*model.Post
	// user_id = userID OR (user_id IN authorIDs AND id IN postIDs)
	err := s.query().NewSelect().Model(&posts).
		Relation("User").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("?TableAlias.user_id = ?", userID).
				WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return q.
						Where("?TableAlias.user_id IN (?)", bun.List(authorIDs)).
						Where("?TableAlias.id IN (?)", bun.List(postIDs))
				})
		}).
		Scan(ctx)

	return posts, err
}

// DraftPosts returns the unpublished posts of a user, the last edited first.
func (s *Store) DraftPosts(ctx context.Context, userID string) ([]*model.Post, error) {
	var posts []*model.Post
	err := s.query().NewSelect().Model(&posts).
		Where("user_id = ?", userID).
		Where("published_at IS NULL").
		OrderExpr("updated_at DESC").
		Scan(ctx)

	return posts, err
}
