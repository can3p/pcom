package model

import (
	"context"
	"github.com/uptrace/bun"
	"time"
)

// MediaKind is the Postgres enum media_kind: what an upload holds.
type MediaKind string

// The values of MediaKind.
const (
	// MediaKindImage is an image, public to anybody with its URL.
	MediaKindImage MediaKind = "image"
	// MediaKindNoteAudio is a note's recording, which only its owner may fetch.
	MediaKindNoteAudio MediaKind = "note_audio"
)

// MediaUpload is a row of media_uploads.
type MediaUpload struct {
	bun.BaseModel `bun:"table:media_uploads"`

	ID            string    `bun:"id,pk"`
	UserID        *string   `bun:"user_id"`
	UploadedFname string    `bun:"uploaded_fname"`
	ContentType   string    `bun:"content_type"`
	CreatedAt     time.Time `bun:"created_at,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,default:now()"`
	RSSFeedID     *string   `bun:"rss_feed_id"`
	Kind          MediaKind `bun:"kind,default:'image'"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *MediaUpload) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
