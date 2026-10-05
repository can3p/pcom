package model

import (
	"context"
	"fmt"
	"github.com/uptrace/bun"
	"slices"
	"time"
)

// PostVisibility is the Postgres enum post_visibility.
type PostVisibility string

// The values of PostVisibility.
const (
	PostVisibilityDirectOnly   PostVisibility = "direct_only"
	PostVisibilitySecondDegree PostVisibility = "second_degree"
	PostVisibilityPublic       PostVisibility = "public"
)

// AllPostVisibility returns every value of PostVisibility, in the enum's order.
func AllPostVisibility() []PostVisibility {
	return []PostVisibility{PostVisibilityDirectOnly, PostVisibilitySecondDegree, PostVisibilityPublic}
}

// IsValid reports an error when e is not a value of PostVisibility.
func (e PostVisibility) IsValid() error {
	if !slices.Contains(AllPostVisibility(), e) {
		return fmt.Errorf("enum PostVisibility: %q is not valid", string(e))
	}

	return nil
}

func (e PostVisibility) String() string {
	return string(e)
}

// NormalizedURL is a row of normalized_urls.
type NormalizedURL struct {
	bun.BaseModel `bun:"table:normalized_urls"`

	ID        string    `bun:"id,pk"`
	URL       string    `bun:"url"`
	CreatedAt time.Time `bun:"created_at"`
	UpdatedAt time.Time `bun:"updated_at"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *NormalizedURL) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// Post is a row of posts.
type Post struct {
	bun.BaseModel `bun:"table:posts"`

	ID               string         `bun:"id,pk"`
	Subject          *string        `bun:"subject"`
	Body             string         `bun:"body"`
	UserID           string         `bun:"user_id"`
	CreatedAt        *time.Time     `bun:"created_at"`
	UpdatedAt        *time.Time     `bun:"updated_at"`
	VisibilityRadius PostVisibility `bun:"visibility_radius"`
	PublishedAt      *time.Time     `bun:"published_at"`
	URLID            *string        `bun:"url_id"`
	RSSItemID        *string        `bun:"rss_item_id"`

	RSSItem  *RSSItem       `bun:"rel:belongs-to,join:rss_item_id=id"`
	URL      *NormalizedURL `bun:"rel:belongs-to,join:url_id=id"`
	User     *User          `bun:"rel:belongs-to,join:user_id=id"`
	PostStat *PostStat      `bun:"rel:has-one,join:id=post_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *Post) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// PostComment is a row of post_comments.
type PostComment struct {
	bun.BaseModel `bun:"table:post_comments"`

	ID              string     `bun:"id,pk"`
	UserID          string     `bun:"user_id"`
	PostID          string     `bun:"post_id"`
	ParentCommentID *string    `bun:"parent_comment_id"`
	Body            string     `bun:"body"`
	CreatedAt       time.Time  `bun:"created_at"`
	UpdatedAt       time.Time  `bun:"updated_at"`
	TopCommentID    string     `bun:"top_comment_id"`
	EditedAt        *time.Time `bun:"edited_at"`

	Post *Post `bun:"rel:belongs-to,join:post_id=id"`
	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *PostComment) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// PostPrompt is a row of post_prompts.
type PostPrompt struct {
	bun.BaseModel `bun:"table:post_prompts"`

	ID          string     `bun:"id,pk"`
	AskerID     string     `bun:"asker_id"`
	RecipientID string     `bun:"recipient_id"`
	Message     string     `bun:"message"`
	DismissedAt *time.Time `bun:"dismissed_at"`
	PostID      *string    `bun:"post_id"`
	CreatedAt   time.Time  `bun:"created_at"`
	UpdatedAt   time.Time  `bun:"updated_at"`

	Asker *User `bun:"rel:belongs-to,join:asker_id=id"`
	Post  *Post `bun:"rel:belongs-to,join:post_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *PostPrompt) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// PostShare is a row of post_shares.
type PostShare struct {
	bun.BaseModel `bun:"table:post_shares"`

	ID        string    `bun:"id,pk"`
	PostID    string    `bun:"post_id"`
	CreatedAt time.Time `bun:"created_at"`
	UpdatedAt time.Time `bun:"updated_at"`

	Post *Post `bun:"rel:belongs-to,join:post_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *PostShare) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// PostStat is a row of post_stats.
type PostStat struct {
	bun.BaseModel `bun:"table:post_stats"`

	ID             string    `bun:"id,pk"`
	PostID         string    `bun:"post_id"`
	CommentsNumber int64     `bun:"comments_number"`
	CreatedAt      time.Time `bun:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at"`

	Post *Post `bun:"rel:belongs-to,join:post_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *PostStat) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
