package reading

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/samber/lo"
)

// FeedItem is one entry of the feed: a post, an RSS item or a comment.
type FeedItem struct {
	Post     *postops.Post
	FeedItem *feedops.RssFeedItem
	Comment  *postops.Comment
}

func (i *FeedItem) AddedToFeedAt() time.Time {
	if i.Post != nil {
		return i.Post.PublishedAt.Time
	}

	if i.FeedItem != nil {
		return i.FeedItem.AddedAt
	}

	return i.Comment.CreatedAt
}

// Feed is the actor's feed. PostsOnly feeds carry the posts and nothing
// else.
type Feed struct {
	PostsOnly         bool
	Items             []*FeedItem
	DirectConnections []*core.User
	OpenPrompts       []*postops.PostPrompt
	// FeedToken is the actor's private RSS feed token, nil when they have none.
	FeedToken *core.UserFeedToken
}

// Feed is what the actor reads at /feed, newest first: the published posts of
// their direct connections, the posts of second-degree connections shared that
// far, their RSS items and the comments on posts they take part in. postsOnly
// stops after the posts, for the private RSS feed.
func (s *Service) Feed(ctx context.Context, actor *core.User, postsOnly bool) (*Feed, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	direct, secondDegree, via, err := graph.DirectAndSecondDegree(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	items, err := s.feedPosts(ctx, actor, direct, secondDegree, via)
	if err != nil {
		return nil, err
	}

	if postsOnly {
		return &Feed{PostsOnly: true, Items: items}, nil
	}

	rssItems, err := s.rssItems(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	items = append(items, rssItems...)

	comments, err := s.feedComments(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	items = append(items, comments...)

	// newest items first
	slices.SortFunc(items, func(a, b *FeedItem) int {
		return b.AddedToFeedAt().Compare(a.AddedToFeedAt())
	})

	out := &Feed{Items: items}

	out.DirectConnections, err = s.store.UsersByIDs(ctx, direct)
	if err != nil {
		return nil, err
	}

	prompts, err := s.store.OpenPromptsFor(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	out.OpenPrompts = lo.Map(prompts, func(p *core.PostPrompt, _ int) *postops.PostPrompt {
		return &postops.PostPrompt{Prompt: p, Author: p.R.Asker, Post: p.R.Post}
	})

	out.FeedToken, err = s.store.FeedTokenForUser(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// PrivateFeed is what a private RSS feed lists: the posts of its owner's feed.
type PrivateFeed struct {
	Owner *core.User
	Posts []*postops.Post
}

// PrivateFeed resolves a private RSS feed token. Anybody who has the token
// may read the feed, so there is no actor.
func (s *Service) PrivateFeed(ctx context.Context, token string) (*PrivateFeed, error) {
	owner, err := s.store.FeedOwner(ctx, token)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	feed, err := s.Feed(ctx, owner, true)
	if err != nil {
		return nil, err
	}

	return &PrivateFeed{
		Owner: owner,
		Posts: lo.Map(feed.Items, func(item *FeedItem, _ int) *postops.Post { return item.Post }),
	}, nil
}

// feedPosts is the published posts of the direct connections, and of the
// second-degree connections those shared that far, each with the direct
// connections it came through.
func (s *Service) feedPosts(ctx context.Context, actor *core.User, direct, secondDegree []string, via map[string][]string) ([]*FeedItem, error) {
	posts, err := s.store.PublishedPostsOfUsers(ctx, direct, secondDegree,
		[]core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic})
	if err != nil {
		return nil, err
	}

	directMap := lo.KeyBy(direct, func(u string) string { return u })
	secondDegreeMap := lo.KeyBy(secondDegree, func(u string) string { return u })

	seenUserIDs := lo.Filter(lo.Uniq(
		lo.Map(posts, func(p *core.Post, _ int) string { return p.UserID }),
	), func(id string, _ int) bool {
		_, ok := secondDegreeMap[id]
		return ok
	})

	viaUserIDs := lo.Uniq(lo.FlatMap(seenUserIDs, func(id string, _ int) []string { return via[id] }))

	viaUsers, err := s.store.UsersByIDsNewestFirst(ctx, viaUserIDs)
	if err != nil {
		return nil, err
	}

	viaUserMap := lo.KeyBy(viaUsers, func(u *core.User) string { return u.ID })

	return lo.Map(posts, func(p *core.Post, _ int) *FeedItem {
		radius := userops.ConnectionRadiusSecondDegree
		var viaUsers []*core.User

		if _, ok := directMap[p.UserID]; ok {
			radius = userops.ConnectionRadiusDirect
		} else {
			viaUsers = lo.Map(via[p.UserID], func(id string, _ int) *core.User { return viaUserMap[id] })
		}

		return &FeedItem{Post: postops.ConstructPost(actor, p, radius, viaUsers, false)}
	}), nil
}

// rssItems is the RSS items in the user's feed they haven't dismissed.
func (s *Service) rssItems(ctx context.Context, userID string) ([]*FeedItem, error) {
	dbItems, err := s.store.UndismissedFeedItems(ctx, userID)
	if err != nil {
		return nil, err
	}

	return lo.Map(dbItems, func(item *core.UserFeedItem, _ int) *FeedItem {
		publishedAt := item.CreatedAt

		if !item.R.RSSItem.PublishedAt.IsZero() {
			publishedAt = item.R.RSSItem.PublishedAt
		}

		return &FeedItem{FeedItem: &feedops.RssFeedItem{
			ID:          item.ID,
			URL:         item.R.URL.URL,
			Title:       item.R.RSSItem.Title,
			Summary:     item.R.RSSItem.SanitizedDescription,
			PublishedAt: publishedAt,
			AddedAt:     item.CreatedAt,
			FeedTitle:   item.R.RSSItem.R.Feed.Title.String,
			FeedURL:     item.R.RSSItem.R.Feed.URL,
		}}
	}), nil
}

// feedComments is the comments others left on the user's own posts, and on
// the posts of their direct connections they have commented on. The
// connection is checked again because the user may have lost it since.
func (s *Service) feedComments(ctx context.Context, userID string) ([]*FeedItem, error) {
	participatedPostIDs, err := s.store.CommentedPostIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	directUserIDs, err := graph.DirectUserIDs(ctx, s.store, userID)
	if err != nil {
		return nil, err
	}

	posts, err := s.store.PostsOfOrAmong(ctx, userID, directUserIDs, participatedPostIDs)
	if err != nil {
		return nil, err
	}

	postMap := lo.KeyBy(posts, func(p *core.Post) string { return p.ID })

	comments, err := s.store.CommentsOnPostsNotBy(ctx, lo.Map(posts, func(p *core.Post, _ int) string { return p.ID }), userID)
	if err != nil {
		return nil, err
	}

	return lo.Map(comments, func(c *core.PostComment, _ int) *FeedItem {
		post := postMap[c.PostID]

		return &FeedItem{
			Comment: &postops.Comment{
				PostComment: c,
				Author:      c.R.User,
				Post:        &postops.Post{Author: post.R.User, Post: post},
			},
		}
	}), nil
}
