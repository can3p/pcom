package repo

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model/core"
)

// ShareOfPost returns a post's share link, or nil when it has none.
func (s *Store) ShareOfPost(ctx context.Context, postID string) (*core.PostShare, error) {
	share, err := core.PostShares(core.PostShareWhere.PostID.EQ(postID)).One(ctx, s.exec)
	return orNil(share, err)
}

// orNil turns "no rows" into a nil result without an error.
func orNil[T any](v *T, err error) (*T, error) {
	if err = notFound(err); errors.Is(err, ErrNotFound) {
		return nil, nil
	}

	return v, err
}
