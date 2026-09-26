package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// InvitationOpt customizes a UserInvitation before it is inserted.
type InvitationOpt func(*core.UserInvitation)

// Sent marks the invitation as already emailed to the given address.
func Sent(email string) InvitationOpt {
	return func(i *core.UserInvitation) {
		i.InvitationEmail = null.StringFrom(email)
		i.InvitationSentAt = null.TimeFrom(time.Now())
	}
}

// Invitation inserts one of userID's invitation slots.
func Invitation(ctx context.Context, exec boil.ContextExecutor, userID string, opts ...InvitationOpt) (*core.UserInvitation, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	i := &core.UserInvitation{
		ID:     id,
		UserID: userID,
	}

	for _, opt := range opts {
		opt(i)
	}

	if err := i.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return i, nil
}

// SignupRequest inserts a pending request to join, with a unique email.
func SignupRequest(ctx context.Context, exec boil.ContextExecutor) (*core.UserSignupRequest, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	n := next()

	r := &core.UserSignupRequest{
		ID:    id,
		Email: fmt.Sprintf("signup%d@example.test", n),
	}

	if err := r.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return r, nil
}

// APIKey issues userID a fresh API key.
func APIKey(ctx context.Context, exec boil.ContextExecutor, userID string) (*core.UserAPIKey, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	key, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	k := &core.UserAPIKey{
		ID:     id,
		APIKey: key.String(),
		UserID: userID,
	}

	if err := k.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return k, nil
}

// UserStyle sets userID's custom profile CSS.
func UserStyle(ctx context.Context, exec boil.ContextExecutor, userID string, css string) (*core.UserStyle, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s := &core.UserStyle{
		ID:     id,
		UserID: userID,
		Styles: css,
	}

	if err := s.Insert(ctx, exec, boil.Infer()); err != nil {
		return nil, err
	}

	return s, nil
}

// SetRegistrationOpen flips the singleton system setting that gates signup.
func SetRegistrationOpen(ctx context.Context, exec boil.ContextExecutor, open bool) error {
	settings, err := core.SystemSettings().One(ctx, exec)
	if err != nil {
		return err
	}

	settings.RegistrationOpen = open

	_, err = settings.Update(ctx, exec, boil.Whitelist(core.SystemSettingColumns.RegistrationOpen))

	return err
}
