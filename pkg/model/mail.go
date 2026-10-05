package model

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/uptrace/bun"
	"slices"
	"time"
)

// OutgoingEmailStatus is the Postgres enum outgoing_email_status.
type OutgoingEmailStatus string

// The values of OutgoingEmailStatus.
const (
	OutgoingEmailStatusNew    OutgoingEmailStatus = "new"
	OutgoingEmailStatusSent   OutgoingEmailStatus = "sent"
	OutgoingEmailStatusFailed OutgoingEmailStatus = "failed"
)

// AllOutgoingEmailStatus returns every value of OutgoingEmailStatus, in the enum's order.
func AllOutgoingEmailStatus() []OutgoingEmailStatus {
	return []OutgoingEmailStatus{OutgoingEmailStatusNew, OutgoingEmailStatusSent, OutgoingEmailStatusFailed}
}

// IsValid reports an error when e is not a value of OutgoingEmailStatus.
func (e OutgoingEmailStatus) IsValid() error {
	if !slices.Contains(AllOutgoingEmailStatus(), e) {
		return fmt.Errorf("enum OutgoingEmailStatus: %q is not valid", string(e))
	}

	return nil
}

func (e OutgoingEmailStatus) String() string {
	return string(e)
}

// OutgoingEmail is a row of outgoing_emails.
type OutgoingEmail struct {
	bun.BaseModel `bun:"table:outgoing_emails"`

	ID             string              `bun:"id,pk"`
	UniqueID       string              `bun:"unique_id"`
	Payload        json.RawMessage     `bun:"payload,type:jsonb"`
	Status         OutgoingEmailStatus `bun:"status"`
	AttemptsNumber int                 `bun:"attempts_number"`
	TryAt          time.Time           `bun:"try_at"`
	SentAt         *time.Time          `bun:"sent_at"`
	CreatedAt      time.Time           `bun:"created_at"`
	UpdatedAt      time.Time           `bun:"updated_at"`
	EmailType      string              `bun:"email_type"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *OutgoingEmail) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
