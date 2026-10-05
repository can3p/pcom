package model

import (
	"context"
	"fmt"
	"github.com/uptrace/bun"
	"slices"
	"time"
)

// ConnectionMediationDecision is the Postgres enum connection_mediation_decision.
type ConnectionMediationDecision string

// The values of ConnectionMediationDecision.
const (
	ConnectionMediationDecisionSigned    ConnectionMediationDecision = "signed"
	ConnectionMediationDecisionDismissed ConnectionMediationDecision = "dismissed"
)

// AllConnectionMediationDecision returns every value of ConnectionMediationDecision, in the enum's order.
func AllConnectionMediationDecision() []ConnectionMediationDecision {
	return []ConnectionMediationDecision{ConnectionMediationDecisionSigned, ConnectionMediationDecisionDismissed}
}

// IsValid reports an error when e is not a value of ConnectionMediationDecision.
func (e ConnectionMediationDecision) IsValid() error {
	if !slices.Contains(AllConnectionMediationDecision(), e) {
		return fmt.Errorf("enum ConnectionMediationDecision: %q is not valid", string(e))
	}

	return nil
}

func (e ConnectionMediationDecision) String() string {
	return string(e)
}

// ConnectionRequestDecision is the Postgres enum connection_request_decision.
type ConnectionRequestDecision string

// The values of ConnectionRequestDecision.
const (
	ConnectionRequestDecisionApproved  ConnectionRequestDecision = "approved"
	ConnectionRequestDecisionDismissed ConnectionRequestDecision = "dismissed"
)

// AllConnectionRequestDecision returns every value of ConnectionRequestDecision, in the enum's order.
func AllConnectionRequestDecision() []ConnectionRequestDecision {
	return []ConnectionRequestDecision{ConnectionRequestDecisionApproved, ConnectionRequestDecisionDismissed}
}

// IsValid reports an error when e is not a value of ConnectionRequestDecision.
func (e ConnectionRequestDecision) IsValid() error {
	if !slices.Contains(AllConnectionRequestDecision(), e) {
		return fmt.Errorf("enum ConnectionRequestDecision: %q is not valid", string(e))
	}

	return nil
}

func (e ConnectionRequestDecision) String() string {
	return string(e)
}

// UserConnection is a row of user_connections.
type UserConnection struct {
	bun.BaseModel `bun:"table:user_connections"`

	ID        string    `bun:"id,pk"`
	User1ID   string    `bun:"user1_id"`
	User2ID   string    `bun:"user2_id"`
	CreatedAt time.Time `bun:"created_at"`
	UpdatedAt time.Time `bun:"updated_at"`

	ConnectionUserConnectionMediationRequest *UserConnectionMediationRequest `bun:"rel:has-one,join:id=connection_id"`
	ConnectionWhitelistedConnection          *WhitelistedConnection          `bun:"rel:has-one,join:id=connection_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserConnection) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserConnectionMediationRequest is a row of user_connection_mediation_requests.
type UserConnectionMediationRequest struct {
	bun.BaseModel `bun:"table:user_connection_mediation_requests"`

	ID              string                     `bun:"id,pk"`
	WhoUserID       string                     `bun:"who_user_id"`
	TargetUserID    string                     `bun:"target_user_id"`
	TargetDecision  *ConnectionRequestDecision `bun:"target_decision"`
	TargetDecidedAt *time.Time                 `bun:"target_decided_at"`
	TargetNote      *string                    `bun:"target_note"`
	ConnectionID    *string                    `bun:"connection_id"`
	CreatedAt       time.Time                  `bun:"created_at"`
	UpdatedAt       time.Time                  `bun:"updated_at"`
	SourceNote      *string                    `bun:"source_note"`

	TargetUser                       *User                     `bun:"rel:belongs-to,join:target_user_id=id"`
	WhoUser                          *User                     `bun:"rel:belongs-to,join:who_user_id=id"`
	MediationUserConnectionMediators []*UserConnectionMediator `bun:"rel:has-many,join:id=mediation_id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *UserConnectionMediationRequest) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}

// UserConnectionMediator is a row of user_connection_mediators.
type UserConnectionMediator struct {
	bun.BaseModel `bun:"table:user_connection_mediators"`

	ID           string                      `bun:"id,pk"`
	MediationID  string                      `bun:"mediation_id"`
	UserID       string                      `bun:"user_id"`
	Decision     ConnectionMediationDecision `bun:"decision"`
	DecidedAt    time.Time                   `bun:"decided_at"`
	MediatorNote *string                     `bun:"mediator_note"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

// WhitelistedConnection is a row of whitelisted_connections.
type WhitelistedConnection struct {
	bun.BaseModel `bun:"table:whitelisted_connections"`

	ID           string    `bun:"id,pk"`
	WhoID        string    `bun:"who_id"`
	AllowsWhoID  string    `bun:"allows_who_id"`
	CreatedAt    time.Time `bun:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at"`
	ConnectionID *string   `bun:"connection_id"`

	AllowsWho *User `bun:"rel:belongs-to,join:allows_who_id=id"`
}

// BeforeAppendModel stamps the row's timestamps on insert and update.
func (m *WhitelistedConnection) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if m != nil {
		stampTimes(query, &m.CreatedAt, &m.UpdatedAt)
	}

	return nil
}
