package testutil

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func CreateUser(ctx context.Context, exec boil.ContextExecutor, email string) (*model.User, error) {
	user := &model.User{
		ID:             uuid.New().String(),
		Email:          email,
		EmailCanonical: pgsession.CanonicalEmail(email),
		Username:       email,
		Timezone:       "UTC",
	}

	return insert(ctx, exec, user)
}

func CreateRSSFeed(ctx context.Context, exec boil.ContextExecutor, url string, title string) (*model.RSSFeed, error) {
	feed := &model.RSSFeed{
		ID:          uuid.New().String(),
		URL:         url,
		Title:       new(title),
		Description: new("Test feed description"),
	}

	return insert(ctx, exec, feed)
}

func CreateUserFeedSubscription(ctx context.Context, exec boil.ContextExecutor, userID string, feedID string) (*model.UserFeedSubscription, error) {
	subscription := &model.UserFeedSubscription{
		ID:     uuid.New().String(),
		UserID: userID,
		FeedID: feedID,
	}

	return insert(ctx, exec, subscription)
}

func CreateRSSItem(ctx context.Context, exec boil.ContextExecutor, feedID string, urlID string, title string, publishedAt time.Time) (*model.RSSItem, error) {
	item := &model.RSSItem{
		ID:                   uuid.New().String(),
		FeedID:               feedID,
		URLID:                urlID,
		GUID:                 uuid.New().String(),
		Title:                title,
		Description:          "Test description",
		SanitizedDescription: "Test description",
		PublishedAt:          publishedAt,
	}

	return insert(ctx, exec, item)
}

func CreateURL(ctx context.Context, exec boil.ContextExecutor, url string) (*model.NormalizedURL, error) {
	urlRecord := &model.NormalizedURL{
		ID:  uuid.New().String(),
		URL: url,
	}

	return insert(ctx, exec, urlRecord)
}

func CreateUserFeedItem(ctx context.Context, exec boil.ContextExecutor, userID string, rssItemID string, urlID string, createdAt time.Time) (*model.UserFeedItem, error) {
	item := &model.UserFeedItem{
		ID:          uuid.New().String(),
		UserID:      userID,
		RSSItemID:   rssItemID,
		URLID:       urlID,
		IsDismissed: false,
		CreatedAt:   createdAt,
	}

	return insert(ctx, exec, item)
}

func GetRSSFeed(ctx context.Context, exec boil.ContextExecutor, feedID string) (*model.RSSFeed, error) {
	feed := &model.RSSFeed{ID: feedID}
	if err := repo.Query(exec).NewSelect().Model(feed).WherePK().Scan(ctx); err != nil {
		return nil, err
	}

	return feed, nil
}

func GetRSSItemsByFeed(ctx context.Context, exec boil.ContextExecutor, feedID string) ([]*model.RSSItem, error) {
	var items []*model.RSSItem
	if err := repo.Query(exec).NewSelect().Model(&items).Where("feed_id = ?", feedID).Scan(ctx); err != nil {
		return nil, err
	}

	return items, nil
}

func GetUserFeedItemsByUser(ctx context.Context, exec boil.ContextExecutor, userID string) ([]*model.UserFeedItem, error) {
	var items []*model.UserFeedItem
	if err := repo.Query(exec).NewSelect().Model(&items).Where("user_id = ?", userID).Scan(ctx); err != nil {
		return nil, err
	}

	return items, nil
}

// insert stores a new row and returns it.
func insert[M any](ctx context.Context, exec boil.ContextExecutor, m *M) (*M, error) {
	if _, err := repo.Query(exec).NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, err
	}

	return m, nil
}
