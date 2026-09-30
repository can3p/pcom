package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// ShareByID returns a share with its post and the post's author loaded
// (share.R.Post, share.R.Post.R.User).
func (s *Store) ShareByID(ctx context.Context, id string) (*core.PostShare, error) {
	share, err := core.PostShares(
		core.PostShareWhere.ID.EQ(id),
		qm.Load(qm.Rels(core.PostShareRels.Post, core.PostRels.User)),
	).One(ctx, s.exec)

	return share, notFound(err)
}

// CreateShare gives a post its share link. A post has at most one, so
// creating it again keeps the existing link.
func (s *Store) CreateShare(ctx context.Context, postID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	share := &core.PostShare{ID: id.String(), PostID: postID}

	return share.Upsert(ctx, s.exec, false, []string{core.PostShareColumns.PostID}, boil.Infer(), boil.Infer())
}

// DeleteShares removes a post's share link, if it has one.
func (s *Store) DeleteShares(ctx context.Context, postID string) error {
	_, err := core.PostShares(core.PostShareWhere.PostID.EQ(postID)).DeleteAll(ctx, s.exec)
	return err
}
