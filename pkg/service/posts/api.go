package posts

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
)

// ListMax is the most posts one call of List returns.
const ListMax = 100

// ListInput selects a page of the actor's posts.
type ListInput struct {
	// UpdatedSince is a Unix time; only posts updated after it are listed.
	UpdatedSince int64
	// Cursor is the Cursor of the previous page.
	Cursor string
	// Limit is the page size, between 1 and ListMax.
	Limit int
}

// Listing is a page of posts, newest first. Cursor is empty on the last page.
type Listing struct {
	Posts  []*model.Post
	Cursor string
}

// List returns a page of the actor's posts, drafts included.
func (s *Service) List(ctx context.Context, actor *model.User, in ListInput) (*Listing, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	switch {
	case in.Limit <= 0:
		in.Limit = 1
	case in.Limit > ListMax:
		in.Limit = ListMax
	}

	page := repo.PostsPage{
		AuthorID: actor.ID,
		Cursor:   in.Cursor,
		// +1 here is to understand whether it makes sense to fill cursor value,
		// we're discarding the last record otherwise
		Limit: in.Limit + 1,
	}

	if in.UpdatedSince > 0 {
		page.UpdatedSince = time.Unix(in.UpdatedSince, 0).UTC()
	}

	posts, err := s.store.PostsByAuthor(ctx, page)
	if err != nil {
		return nil, err
	}

	if len(posts) == 0 {
		return &Listing{}, nil
	}

	listing := &Listing{Posts: posts}

	if len(posts) > in.Limit {
		listing.Posts = posts[:len(posts)-1]
		listing.Cursor = posts[in.Limit-1].ID
	}

	return listing, nil
}
