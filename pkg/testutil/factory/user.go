package factory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// aboutTexts carries WithProfileAbout's text from the option to User, which
// saves it once the user row exists. UserOpt only sees the user.
var aboutTexts sync.Map

// UserOpt customizes a User before it is inserted.
type UserOpt func(*model.User)

// WithUsername overrides the made-up username.
func WithUsername(name string) UserOpt {
	return func(u *model.User) {
		u.Username = name
	}
}

// WithEmail overrides the made-up email.
func WithEmail(email string) UserOpt {
	return func(u *model.User) {
		u.Email = email
	}
}

// WithVisibility overrides the user's default profile visibility.
func WithVisibility(v model.ProfileVisibility) UserOpt {
	return func(u *model.User) {
		u.ProfileVisibility = v
	}
}

// WithProfileAbout gives the user an "About" text, saved after the user is
// inserted.
func WithProfileAbout(about string) UserOpt {
	return func(u *model.User) {
		aboutTexts.Store(u, about)
	}
}

// Unconfirmed leaves the user's email unconfirmed, as right after signup.
func Unconfirmed() UserOpt {
	return func(u *model.User) {
		u.EmailConfirmedAt = nil
	}
}

// User inserts a confirmed user with a unique email/username pair
// (e.g. user7@example.test / user7) in the UTC timezone.
func User(ctx context.Context, exec boil.ContextExecutor, opts ...UserOpt) (*model.User, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	u := &model.User{
		ID:               id,
		Email:            fmt.Sprintf("user%d@example.test", n),
		Username:         fmt.Sprintf("user%d", n),
		Timezone:         "UTC",
		EmailConfirmedAt: new(time.Now()),
	}

	for _, opt := range opts {
		opt(u)
	}

	u.EmailCanonical = pgsession.CanonicalEmail(u.Email)

	if err := insert(ctx, exec, u); err != nil {
		return nil, err
	}

	if about, ok := aboutTexts.LoadAndDelete(u); ok {
		if err := repo.Using(exec).SaveProfileAbout(ctx, u.ID, about.(string)); err != nil {
			return nil, err
		}
	}

	return u, nil
}

// Connect makes aID and bID direct connections, inserting both directed
// rows the way repo.CreateConnection does.
func Connect(ctx context.Context, exec boil.ContextExecutor, aID, bID string) (*model.UserConnection, *model.UserConnection, error) {
	return repo.Using(exec).CreateConnection(ctx, aID, bID)
}

// Whitelist lets allowsWhoID connect to whoID without mediation.
func Whitelist(ctx context.Context, exec boil.ContextExecutor, whoID, allowsWhoID string) (*model.WhitelistedConnection, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	wl := &model.WhitelistedConnection{
		ID:          id,
		WhoID:       whoID,
		AllowsWhoID: allowsWhoID,
	}

	return insertRow(ctx, exec, wl)
}

// MediationRequestOpt customizes a mediation request before it is inserted.
type MediationRequestOpt func(*model.UserConnectionMediationRequest)

// WithSourceNote sets the note the requester attaches to the request.
func WithSourceNote(note string) MediationRequestOpt {
	return func(r *model.UserConnectionMediationRequest) {
		r.SourceNote = new(note)
	}
}

// MediationRequest records whoID asking to be connected to targetID through
// a common connection.
func MediationRequest(ctx context.Context, exec boil.ContextExecutor, whoID, targetID string, opts ...MediationRequestOpt) (*model.UserConnectionMediationRequest, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	r := &model.UserConnectionMediationRequest{
		ID:           id,
		WhoUserID:    whoID,
		TargetUserID: targetID,
	}

	for _, opt := range opts {
		opt(r)
	}

	return insertRow(ctx, exec, r)
}

// MediatorDecision records mediatorID's decision on the mediation request
// requestID.
func MediatorDecision(ctx context.Context, exec boil.ContextExecutor, requestID, mediatorID string, decision model.ConnectionMediationDecision) (*model.UserConnectionMediator, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	m := &model.UserConnectionMediator{
		ID:          id,
		MediationID: requestID,
		UserID:      mediatorID,
		Decision:    decision,
		DecidedAt:   time.Now(),
	}

	return insertRow(ctx, exec, m)
}
