package testutil

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func CreateUser(ctx context.Context, exec boil.ContextExecutor, email string) (*core.User, error) {
	user := &core.User{
		ID:       uuid.New().String(),
		Email:    email,
		Username: email,
		Timezone: "UTC",
	}

	if err := user.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return user, nil
}

func CreateRSSFeed(ctx context.Context, exec boil.ContextExecutor, url string, title string) (*core.RSSFeed, error) {
	feed := &core.RSSFeed{
		ID:          uuid.New().String(),
		URL:         url,
		Title:       null.StringFrom(title),
		Description: null.StringFrom("Test feed description"),
	}

	if err := feed.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return feed, nil
}

func CreateUserFeedSubscription(ctx context.Context, exec boil.ContextExecutor, userID string, feedID string) (*core.UserFeedSubscription, error) {
	subscription := &core.UserFeedSubscription{
		ID:     uuid.New().String(),
		UserID: userID,
		FeedID: feedID,
	}

	if err := subscription.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return subscription, nil
}

func CreateRSSItem(ctx context.Context, exec boil.ContextExecutor, feedID string, urlID string, title string, publishedAt time.Time) (*core.RSSItem, error) {
	item := &core.RSSItem{
		ID:                   uuid.New().String(),
		FeedID:               feedID,
		URLID:                urlID,
		GUID:                 uuid.New().String(),
		Title:                title,
		Description:          "Test description",
		SanitizedDescription: "Test description",
		PublishedAt:          publishedAt,
	}

	if err := item.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return item, nil
}

func CreateURL(ctx context.Context, exec boil.ContextExecutor, url string) (*core.NormalizedURL, error) {
	urlRecord := &core.NormalizedURL{
		ID:  uuid.New().String(),
		URL: url,
	}

	if err := urlRecord.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return urlRecord, nil
}

func CreateUserFeedItem(ctx context.Context, exec boil.ContextExecutor, userID string, rssItemID string, urlID string, createdAt time.Time) (*core.UserFeedItem, error) {
	item := &core.UserFeedItem{
		ID:          uuid.New().String(),
		UserID:      userID,
		RSSItemID:   rssItemID,
		URLID:       urlID,
		IsDismissed: false,
		CreatedAt:   createdAt,
	}

	if err := item.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return item, nil
}

func GetRSSFeed(ctx context.Context, exec boil.ContextExecutor, feedID string) (*core.RSSFeed, error) {
	return core.FindRSSFeed(ctx, exec, feedID)
}

func GetRSSItemsByFeed(ctx context.Context, exec boil.ContextExecutor, feedID string) (core.RSSItemSlice, error) {
	return core.RSSItems(core.RSSItemWhere.FeedID.EQ(feedID)).All(ctx, exec)
}

func GetUserFeedItemsByUser(ctx context.Context, exec boil.ContextExecutor, userID string) (core.UserFeedItemSlice, error) {
	return core.UserFeedItems(core.UserFeedItemWhere.UserID.EQ(userID)).All(ctx, exec)
}
