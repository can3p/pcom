package repo

import (
	"context"

	"github.com/can3p/pcom/pkg/model/core"
)

// PostByID returns a post, draft or published.
func (s *Store) PostByID(ctx context.Context, id string) (*core.Post, error) {
	post, err := core.FindPost(ctx, s.exec, id)
	return post, notFound(err)
}
