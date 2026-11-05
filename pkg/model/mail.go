package model

import (
	"context"
	"encoding/json"
	"github.com/uptrace/bun"
	"time"
)

// SentEmail is a row of sent_emails: a mail pcom sent and what it sent.
// UniqueID is the SHA-1 uuid of the mail's unique id string; with EmailType
// it drops a repeat for good.
type SentEmail struct {
	bun.BaseModel `bun:"table:sent_emails"`

	ID        string          `bun:"id,pk"`
	UniqueID  string          `bun:"unique_id"`
	EmailType string          `bun:"email_type"`
	Payload   json.RawMessage `bun:"payload,type:jsonb"`
	SentAt    time.Time       `bun:"sent_at"`
	CreatedAt time.Time       `bun:"created_at"`
}

// BeforeAppendModel stamps the row's creation time on insert.
func (m *SentEmail) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, nil)
	}

	return nil
}
