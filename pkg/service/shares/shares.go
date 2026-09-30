// Package shares manages share links: a secret URL that shows one published
// post to anybody who has it, logged in or not.
package shares

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
)

type Service struct {
	store *repo.Store
}

func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// Shared is what a share link shows.
type Shared struct {
	Post   *core.Post
	Author *core.User
}

// Get resolves a share link. Anybody may follow one, so there is no actor.
// A link to a post that is a draft again resolves to nothing.
func (s *Service) Get(ctx context.Context, shareID string) (*Shared, error) {
	share, err := s.store.ShareByID(ctx, shareID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	post := share.R.Post
	if post.PublishedAt.IsZero() {
		return nil, service.ErrNotFound
	}

	return &Shared{Post: post, Author: post.R.User}, nil
}

// Create gives the actor's published post a share link, or keeps the one it
// has.
func (s *Service) Create(ctx context.Context, actor *core.User, postID string) error {
	post, err := s.authorsPost(ctx, actor, postID)
	if err != nil {
		return err
	}

	if post.PublishedAt.IsZero() {
		return service.Invalid("postId", "Cannot share a link for draft")
	}

	return s.store.CreateShare(ctx, post.ID)
}

// Delete removes the share link of the actor's post, so the old URL stops
// working.
func (s *Service) Delete(ctx context.Context, actor *core.User, postID string) error {
	post, err := s.authorsPost(ctx, actor, postID)
	if err != nil {
		return err
	}

	return s.store.DeleteShares(ctx, post.ID)
}

// authorsPost loads a post the actor may manage the share link of: only its
// author may, as postops.GetPostCapabilities' CanShare says.
func (s *Service) authorsPost(ctx context.Context, actor *core.User, postID string) (*core.Post, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	post, err := s.store.PostByID(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if post.UserID != actor.ID {
		return nil, service.ErrForbidden
	}

	return post, nil
}
