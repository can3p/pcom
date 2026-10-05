package model

import (
	"context"
	"github.com/uptrace/bun"
	"time"
)

// UserInvitation is a row of user_invitations.
type UserInvitation struct {
	bun.BaseModel `bun:"table:user_invitations"`

	ID               string     `bun:"id,pk"`
	UserID           string     `bun:"user_id"`
	InvitationEmail  *string    `bun:"invitation_email"`
	InvitationSentAt *time.Time `bun:"invitation_sent_at"`
	CreatedAt        *time.Time `bun:"created_at"`
	UpdatedAt        *time.Time `bun:"updated_at"`
	CreatedUserID    *string    `bun:"created_user_id"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserInvitation) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserSignupRequest is a row of user_signup_requests.
type UserSignupRequest struct {
	bun.BaseModel `bun:"table:user_signup_requests"`

	ID                 string     `bun:"id,pk"`
	Email              string     `bun:"email"`
	Reason             *string    `bun:"reason"`
	SignupAttribution  *string    `bun:"signup_attribution"`
	CreatedUserID      *string    `bun:"created_user_id"`
	VerificationSentAt *time.Time `bun:"verification_sent_at"`
	EmailConfirmedAt   *time.Time `bun:"email_confirmed_at"`
	CreatedAt          *time.Time `bun:"created_at"`
	UpdatedAt          *time.Time `bun:"updated_at"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserSignupRequest) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
