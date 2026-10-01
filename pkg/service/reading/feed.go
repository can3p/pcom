package reading

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/feeds"
	"github.com/can3p/pcom/pkg/service/graph"
	"github.com/samber/lo"
)

// FeedItem is one entry of the feed: a post, an RSS item or a comment.
type FeedItem struct {
	Post     *postops.Post
	FeedItem *feeds.RssFeedItem
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

// Feed is a page of the actor's feed. Only the first page carries the
// connections, prompts and feed token. Next is the cursor of the next page,
// empty on the last one.
type Feed struct {
	Items             []*FeedItem
	Next              string
	DirectConnections []*core.User
	OpenPrompts       []*postops.PostPrompt
	// FeedToken is the actor's private RSS feed token, nil when they have none.
	FeedToken *core.UserFeedToken
}

// Feed is the page after cursor (empty for the first) of what the actor
// reads at /feed, newest first: the published posts of their direct
// connections, the posts of second-degree connections shared that far, their
// RSS items and the comments on posts they take part in.
func (s *Service) Feed(ctx context.Context, actor *core.User, cursor string) (*Feed, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	after, err := ParseCursor(cursor)
	if err != nil {
		return nil, err
	}

	page := after.page(s.pageSize)

	items, err := s.feedPosts(ctx, actor, page)
	if err != nil {
		return nil, err
	}

	rssItems, err := s.rssItems(ctx, actor.ID, page)
	if err != nil {
		return nil, err
	}

	items = append(items, rssItems...)

	comments, err := s.feedComments(ctx, actor.ID, page)
	if err != nil {
		return nil, err
	}

	items = append(items, comments...)

	slices.SortFunc(items, compareItems)

	out := &Feed{}
	out.Items, out.Next = cut(items, s.pageSize, cursorOf)

	if cursor != "" {
		return out, nil
	}

	direct, err := graph.DirectUserIDs(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

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
// may read the feed, so there is no actor. It lists the newest rss limit
// posts of the owner's feed.
func (s *Service) PrivateFeed(ctx context.Context, token string) (*PrivateFeed, error) {
	owner, err := s.store.FeedTokenOwner(ctx, token)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	items, err := s.feedPosts(ctx, owner, repo.Page{Limit: s.rssLimit})
	if err != nil {
		return nil, err
	}

	return &PrivateFeed{
		Owner: owner,
		Posts: lo.Map(items, func(item *FeedItem, _ int) *postops.Post { return item.Post }),
	}, nil
}

// feedPosts is a page of the published posts of the actor's direct
// connections, and of the second-degree connections those shared that far,
// each with the direct connections it came through.
func (s *Service) feedPosts(ctx context.Context, actor *core.User, page repo.Page) ([]*FeedItem, error) {
	direct, secondDegree, via, err := graph.DirectAndSecondDegree(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	posts, err := s.store.PublishedPostsOfUsers(ctx, direct, secondDegree,
		[]core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}, page)
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
		radius := graph.RadiusSecondDegree
		var viaUsers []*core.User

		if _, ok := directMap[p.UserID]; ok {
			radius = graph.RadiusDirect
		} else {
			viaUsers = lo.Map(via[p.UserID], func(id string, _ int) *core.User { return viaUserMap[id] })
		}

		return &FeedItem{Post: postops.ConstructPost(actor, p, radius, viaUsers, false)}
	}), nil
}

// rssItems is a page of the RSS items in the user's feed they haven't
// dismissed.
func (s *Service) rssItems(ctx context.Context, userID string, page repo.Page) ([]*FeedItem, error) {
	dbItems, err := s.store.UndismissedFeedItems(ctx, userID, page)
	if err != nil {
		return nil, err
	}

	return lo.Map(dbItems, func(item *core.UserFeedItem, _ int) *FeedItem {
		publishedAt := item.CreatedAt

		if !item.R.RSSItem.PublishedAt.IsZero() {
			publishedAt = item.R.RSSItem.PublishedAt
		}

		return &FeedItem{FeedItem: &feeds.RssFeedItem{
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

// feedComments is a page of the comments others left on the user's own posts, and on
// the posts of their direct connections they have commented on. The
// connection is checked again because the user may have lost it since.
func (s *Service) feedComments(ctx context.Context, userID string, page repo.Page) ([]*FeedItem, error) {
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

	comments, err := s.store.CommentsOnPostsNotBy(ctx, lo.Map(posts, func(p *core.Post, _ int) string { return p.ID }), userID, page)
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
