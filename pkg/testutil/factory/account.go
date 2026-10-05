package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// InvitationOpt customizes a UserInvitation before it is inserted.
type InvitationOpt func(*model.UserInvitation)

// Sent marks the invitation as already emailed to the given address.
func Sent(email string) InvitationOpt {
	return func(i *model.UserInvitation) {
		i.InvitationEmail = new(email)
		i.InvitationSentAt = new(time.Now())
	}
}

// UsedBy marks the invitation as accepted by createdUserID.
func UsedBy(createdUserID string) InvitationOpt {
	return func(i *model.UserInvitation) {
		i.CreatedUserID = new(createdUserID)
	}
}

// Invitation inserts one of userID's invitation slots.
func Invitation(ctx context.Context, exec boil.ContextExecutor, userID string, opts ...InvitationOpt) (*model.UserInvitation, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	i := &model.UserInvitation{
		ID:     id,
		UserID: userID,
	}

	for _, opt := range opts {
		opt(i)
	}

	return insertRow(ctx, exec, i)
}

// SignupRequestOpt customizes a UserSignupRequest before it is inserted.
type SignupRequestOpt func(*model.UserSignupRequest)

// EmailConfirmed marks the request's email as already confirmed through
// /confirm_waiting_list/:id.
func EmailConfirmed() SignupRequestOpt {
	return func(r *model.UserSignupRequest) {
		r.EmailConfirmedAt = new(time.Now())
	}
}

// SignupRequest inserts a pending request to join, with a unique email.
func SignupRequest(ctx context.Context, exec boil.ContextExecutor, opts ...SignupRequestOpt) (*model.UserSignupRequest, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	r := &model.UserSignupRequest{
		ID:    id,
		Email: fmt.Sprintf("signup%d@example.test", n),
	}

	for _, opt := range opts {
		opt(r)
	}

	return insertRow(ctx, exec, r)
}

// APIKeyOpt customizes a UserAPIKey before it is inserted.
type APIKeyOpt func(*model.UserAPIKey)

// WithAPIKey sets a fixed key value instead of a random one.
func WithAPIKey(key string) APIKeyOpt {
	return func(k *model.UserAPIKey) {
		k.APIKey = key
	}
}

// APIKey issues userID a fresh API key, or the one WithAPIKey gives.
func APIKey(ctx context.Context, exec boil.ContextExecutor, userID string, opts ...APIKeyOpt) (*model.UserAPIKey, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	key, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	k := &model.UserAPIKey{
		ID:     id,
		APIKey: key.String(),
		UserID: userID,
	}

	for _, opt := range opts {
		opt(k)
	}

	return insertRow(ctx, exec, k)
}

// UserStyle sets userID's custom profile CSS.
func UserStyle(ctx context.Context, exec boil.ContextExecutor, userID string, css string) (*model.UserStyle, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &model.UserStyle{
		ID:     id,
		UserID: userID,
		Styles: css,
	}

	return insertRow(ctx, exec, s)
}

// SetRegistrationOpen flips the singleton system setting that gates signup.
func SetRegistrationOpen(ctx context.Context, exec boil.ContextExecutor, open bool) error {
	settings := new(model.SystemSetting)
	if err := repo.Query(exec).NewSelect().Model(settings).Limit(1).Scan(ctx); err != nil {
		return err
	}

	settings.RegistrationOpen = open

	_, err := repo.Query(exec).NewUpdate().Model(settings).Column("registration_open").WherePK().Exec(ctx)

	return err
}
