package model

import (
	"context"
	"fmt"
	"github.com/uptrace/bun"
	"slices"
	"time"
)

// ProfileVisibility is the Postgres enum profile_visibility.
type ProfileVisibility string

// The values of ProfileVisibility.
const (
	ProfileVisibilityConnections     ProfileVisibility = "connections"
	ProfileVisibilityRegisteredUsers ProfileVisibility = "registered_users"
	ProfileVisibilityPublic          ProfileVisibility = "public"
)

// AllProfileVisibility returns every value of ProfileVisibility, in the enum's order.
func AllProfileVisibility() []ProfileVisibility {
	return []ProfileVisibility{ProfileVisibilityConnections, ProfileVisibilityRegisteredUsers, ProfileVisibilityPublic}
}

// IsValid reports an error when e is not a value of ProfileVisibility.
func (e ProfileVisibility) IsValid() error {
	if !slices.Contains(AllProfileVisibility(), e) {
		return fmt.Errorf("enum ProfileVisibility: %q is not valid", string(e))
	}

	return nil
}

func (e ProfileVisibility) String() string {
	return string(e)
}

// LoginAttempt is a row of login_attempts.
type LoginAttempt struct {
	bun.BaseModel `bun:"table:login_attempts"`

	ID         string     `bun:"id,pk"`
	UserID     *string    `bun:"user_id"`
	CodeHash   *string    `bun:"code_hash"`
	ReturnURL  string     `bun:"return_url,default:''"`
	WrongTries int        `bun:"wrong_tries,default:0"`
	ExpiresAt  time.Time  `bun:"expires_at"`
	UsedAt     *time.Time `bun:"used_at"`
	CreatedAt  time.Time  `bun:"created_at,default:now()"`
	UpdatedAt  time.Time  `bun:"updated_at,default:now()"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *LoginAttempt) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// User is a row of users.
type User struct {
	bun.BaseModel `bun:"table:users"`

	ID                string            `bun:"id,pk"`
	Email             string            `bun:"email"`
	CreatedAt         *time.Time        `bun:"created_at,default:now()"`
	UpdatedAt         *time.Time        `bun:"updated_at,default:now()"`
	Timezone          string            `bun:"timezone"`
	EmailConfirmedAt  *time.Time        `bun:"email_confirmed_at"`
	EmailConfirmSeed  *string           `bun:"email_confirm_seed"`
	SignupAttribution *string           `bun:"signup_attribution"`
	Pwdhash           *string           `bun:"pwdhash"`
	Username          string            `bun:"username"`
	ProfileVisibility ProfileVisibility `bun:"profile_visibility,default:'registered_users'"`
	EmailCanonical    string            `bun:"email_canonical"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *User) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserAPIKey is a row of user_api_keys.
type UserAPIKey struct {
	bun.BaseModel `bun:"table:user_api_keys"`

	ID        string    `bun:"id,pk"`
	APIKey    string    `bun:"api_key"`
	UserID    string    `bun:"user_id"`
	CreatedAt time.Time `bun:"created_at"`
	UpdatedAt time.Time `bun:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserAPIKey) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserProfile is a row of user_profiles.
type UserProfile struct {
	bun.BaseModel `bun:"table:user_profiles"`

	UserID    string    `bun:"user_id,pk"`
	About     string    `bun:"about"`
	UpdatedAt time.Time `bun:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserProfile) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, nil, &m.UpdatedAt)
	}

	return nil
}

// UserStyle is a row of user_styles.
type UserStyle struct {
	bun.BaseModel `bun:"table:user_styles"`

	ID        string     `bun:"id,pk"`
	UserID    string     `bun:"user_id"`
	Styles    string     `bun:"styles"`
	CreatedAt *time.Time `bun:"created_at"`
	UpdatedAt *time.Time `bun:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserStyle) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
