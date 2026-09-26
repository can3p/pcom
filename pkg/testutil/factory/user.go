package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// UserOpt customizes a User before it is inserted.
type UserOpt func(*core.User)

// WithPassword sets a login password: it hashes it the way the app does and
// marks the user's email confirmed.
func WithPassword(pw string) UserOpt {
	return func(u *core.User) {
		u.Pwdhash = null.StringFrom(pgsession.HashUserPwd(u.Email, pw))
		u.EmailConfirmedAt = null.TimeFrom(time.Now())
	}
}

// WithVisibility overrides the user's default profile visibility.
func WithVisibility(v core.ProfileVisibility) UserOpt {
	return func(u *core.User) {
		u.ProfileVisibility = v
	}
}

// User inserts a confirmed user with a unique email/username pair
// (e.g. user7@example.test / user7) in the UTC timezone.
func User(ctx context.Context, exec boil.ContextExecutor, opts ...UserOpt) (*core.User, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	u := &core.User{
		ID:               id,
		Email:            fmt.Sprintf("user%d@example.test", n),
		Username:         fmt.Sprintf("user%d", n),
		Timezone:         "UTC",
		EmailConfirmedAt: null.TimeFrom(time.Now()),
	}

	for _, opt := range opts {
		opt(u)
	}

	if err := u.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return u, nil
}

// Connect makes aID and bID direct connections, inserting both directed
// rows the way userops.CreateConnection does.
func Connect(ctx context.Context, exec boil.ContextExecutor, aID, bID string) (*core.UserConnection, *core.UserConnection, error) {
	return userops.CreateConnection(ctx, exec, aID, bID)
}

// Whitelist lets allowsWhoID connect to whoID without mediation.
func Whitelist(ctx context.Context, exec boil.ContextExecutor, whoID, allowsWhoID string) (*core.WhitelistedConnection, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	wl := &core.WhitelistedConnection{
		ID:          id,
		WhoID:       whoID,
		AllowsWhoID: allowsWhoID,
	}

	if err := wl.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return wl, nil
}

// MediationRequestOpt customizes a mediation request before it is inserted.
type MediationRequestOpt func(*core.UserConnectionMediationRequest)

// WithSourceNote sets the note the requester attaches to the request.
func WithSourceNote(note string) MediationRequestOpt {
	return func(r *core.UserConnectionMediationRequest) {
		r.SourceNote = null.StringFrom(note)
	}
}

// MediationRequest records whoID asking to be connected to targetID through
// a common connection.
func MediationRequest(ctx context.Context, exec boil.ContextExecutor, whoID, targetID string, opts ...MediationRequestOpt) (*core.UserConnectionMediationRequest, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	r := &core.UserConnectionMediationRequest{
		ID:           id,
		WhoUserID:    whoID,
		TargetUserID: targetID,
	}

	for _, opt := range opts {
		opt(r)
	}

	if err := r.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return r, nil
}

// MediatorDecision records mediatorID's decision on the mediation request
// requestID.
func MediatorDecision(ctx context.Context, exec boil.ContextExecutor, requestID, mediatorID string, decision core.ConnectionMediationDecision) (*core.UserConnectionMediator, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	m := &core.UserConnectionMediator{
		ID:          id,
		MediationID: requestID,
		UserID:      mediatorID,
		Decision:    decision,
		DecidedAt:   time.Now(),
	}

	if err := m.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return m, nil
}
