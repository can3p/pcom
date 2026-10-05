package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

// ShareByID returns a share with its post and the post's author loaded
// (share.Post, share.Post.User).
func (s *Store) ShareByID(ctx context.Context, id string) (*model.PostShare, error) {
	share := new(model.PostShare)
	err := s.query().NewSelect().Model(share).
		Relation("Post.User").
		Where("?TableAlias.id = ?", id).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, notFound(err)
	}

	return share, nil
}

// CreateShare gives a post its share link. A post has at most one, so
// creating it again keeps the existing link.
func (s *Store) CreateShare(ctx context.Context, postID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	share := &model.PostShare{ID: id.String(), PostID: postID}

	_, err = s.query().NewInsert().Model(share).On("CONFLICT (post_id) DO NOTHING").Exec(ctx)

	return err
}

// DeleteShares removes a post's share link, if it has one.
func (s *Store) DeleteShares(ctx context.Context, postID string) error {
	_, err := s.query().NewDelete().Model((*model.PostShare)(nil)).Where("post_id = ?", postID).Exec(ctx)
	return err
}

// ShareOfPost returns a post's share link, or nil when it has none.
func (s *Store) ShareOfPost(ctx context.Context, postID string) (*model.PostShare, error) {
	share := new(model.PostShare)
	if err := s.query().NewSelect().Model(share).Where("post_id = ?", postID).Limit(1).Scan(ctx); err != nil {
		return orNil[model.PostShare](nil, err)
	}

	return share, nil
}
