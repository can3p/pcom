package accounts

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
)

// WithJobs sets the queue the service enqueues its background jobs on.
func WithJobs(q repo.JobQueue) Option {
	return func(s *Service) {
		s.jobs = q
	}
}

// RegisterJobs registers the workers of the service's job kinds on r.
func (s *Service) RegisterJobs(r *jobs.Registry) {
	jobs.RegisterMail(r, s.RenderInvitation)
	jobs.RegisterMail(r, s.RenderAdminNewUser)
	jobs.RegisterMail(r, s.RenderAdminThrowAwayEmail)
	jobs.RegisterMail(r, s.RenderConfirmWaitingList)
	jobs.RegisterMail(r, s.RenderAdminNewWaitingListMember)
	jobs.RegisterMail(r, s.RenderLoginCode)
	jobs.RegisterMail(r, s.RenderConfirmSignup)
	jobs.RegisterMail(r, s.RenderAdminSignupConfirmed)
	jobs.RegisterMail(r, s.RenderAdminPageFailure)
}

// InvitationMailArgs is the invitation mail: the invitation and the address
// it was sent to.
type InvitationMailArgs struct {
	InviteID string `json:"invite_id"`
	To       string `json:"to"`
}

// Kind names the job kind.
func (InvitationMailArgs) Kind() string { return "mail.user_invitation" }

// AdminNewUserMailArgs tells the admin about a new account.
type AdminNewUserMailArgs struct {
	UserID string `json:"user_id"`
}

// Kind names the job kind.
func (AdminNewUserMailArgs) Kind() string { return "mail.admin_new_user" }

// AdminThrowAwayEmailMailArgs tells the admin about a signup attempt with a
// throwaway email address.
type AdminThrowAwayEmailMailArgs struct {
	Email string `json:"email"`
}

// Kind names the job kind.
func (AdminThrowAwayEmailMailArgs) Kind() string { return "mail.throw_away_email_signup" }

// ConfirmWaitingListMailArgs asks a waiting list entry to confirm its
// address.
type ConfirmWaitingListMailArgs struct {
	RequestID string `json:"request_id"`
}

// Kind names the job kind.
func (ConfirmWaitingListMailArgs) Kind() string { return "mail.waiting_list_confirm" }

// AdminNewWaitingListMemberMailArgs tells the admin about a waiting list
// entry.
type AdminNewWaitingListMemberMailArgs struct {
	RequestID string `json:"request_id"`
}

// Kind names the job kind.
func (AdminNewWaitingListMemberMailArgs) Kind() string { return "mail.new_waiting_list_member" }

// LoginCodeMailArgs mails the code of a login attempt. The code is not in
// the database in the clear, so the args carry it.
type LoginCodeMailArgs struct {
	AttemptID string `json:"attempt_id"`
	UserID    string `json:"user_id"`
	Code      string `json:"code"`
}

// Kind names the job kind.
func (LoginCodeMailArgs) Kind() string { return "mail.login_code" }

// ConfirmSignupMailArgs mails the code that confirms a new account's
// address.
type ConfirmSignupMailArgs struct {
	AttemptID string `json:"attempt_id"`
	UserID    string `json:"user_id"`
	Code      string `json:"code"`
}

// Kind names the job kind.
func (ConfirmSignupMailArgs) Kind() string { return "mail.confirm_signup" }

// AdminSignupConfirmedMailArgs tells the admin that an account confirmed its
// address.
type AdminSignupConfirmedMailArgs struct {
	UserID string `json:"user_id"`
}

// Kind names the job kind.
func (AdminSignupConfirmedMailArgs) Kind() string { return "mail.signup_confirmed" }

// AdminPageFailureMailArgs is the panic alert to the admin: the report of the
// request, the panic and its stack, and who hit the page (empty for a visitor
// who is not logged in).
type AdminPageFailureMailArgs struct {
	Report string `json:"report"`
	UserID string `json:"user_id"`
}

// Kind names the job kind.
func (AdminPageFailureMailArgs) Kind() string { return "mail.panic_notification" }

// gone turns a row that no longer exists into "nothing to send".
func gone[T any](v T, err error) (T, bool, error) {
	if errors.Is(err, repo.ErrNotFound) {
		return v, true, nil
	}

	return v, false, err
}

// RenderInvitation builds the invitation mail.
func (s *Service) RenderInvitation(ctx context.Context, a InvitationMailArgs) (*mail.Envelope, error) {
	invite, missing, err := gone(s.store.OpenInvitationByID(ctx, a.InviteID))
	if missing || err != nil {
		return nil, err
	}

	return mail.Invitation(s.ident.Site, s.ident.From, invite, a.To), nil
}

// RenderAdminNewUser builds the admin's new account notification.
func (s *Service) RenderAdminNewUser(ctx context.Context, a AdminNewUserMailArgs) (*mail.Envelope, error) {
	user, missing, err := gone(s.store.UserByID(ctx, a.UserID))
	if missing || err != nil {
		return nil, err
	}

	return mail.AdminNewUser(s.ident.Site, s.ident.From, s.ident.AdminAddress, user), nil
}

// RenderAdminThrowAwayEmail builds the admin's throwaway email notification.
func (s *Service) RenderAdminThrowAwayEmail(_ context.Context, a AdminThrowAwayEmailMailArgs) (*mail.Envelope, error) {
	return mail.AdminThrowAwayEmailSignupAttempt(s.ident.Site, s.ident.From, s.ident.AdminAddress, a.Email), nil
}

// RenderConfirmWaitingList builds the mail that asks a waiting list entry to
// confirm.
func (s *Service) RenderConfirmWaitingList(ctx context.Context, a ConfirmWaitingListMailArgs) (*mail.Envelope, error) {
	request, missing, err := gone(s.store.SignupRequestByID(ctx, a.RequestID))
	if missing || err != nil {
		return nil, err
	}

	return mail.ConfirmWaitingList(s.ident.Site, s.ident.From, request), nil
}

// RenderAdminNewWaitingListMember builds the admin's waiting list
// notification.
func (s *Service) RenderAdminNewWaitingListMember(ctx context.Context, a AdminNewWaitingListMemberMailArgs) (*mail.Envelope, error) {
	request, missing, err := gone(s.store.SignupRequestByID(ctx, a.RequestID))
	if missing || err != nil {
		return nil, err
	}

	return mail.AdminNewWaitingListMember(s.ident.Site, s.ident.From, s.ident.AdminAddress, request), nil
}

// RenderLoginCode builds the mail with a login code.
func (s *Service) RenderLoginCode(ctx context.Context, a LoginCodeMailArgs) (*mail.Envelope, error) {
	user, missing, err := gone(s.store.UserByID(ctx, a.UserID))
	if missing || err != nil {
		return nil, err
	}

	return mail.LoginCode(s.ident.Site, s.ident.From, a.AttemptID, user.Email, a.Code, s.login.CodeLifetime), nil
}

// RenderConfirmSignup builds the mail with the code that confirms a new
// account.
func (s *Service) RenderConfirmSignup(ctx context.Context, a ConfirmSignupMailArgs) (*mail.Envelope, error) {
	user, missing, err := gone(s.store.UserByID(ctx, a.UserID))
	if missing || err != nil {
		return nil, err
	}

	return mail.ConfirmSignup(s.ident.Site, s.ident.From, a.AttemptID, user.Email, a.Code, s.login.CodeLifetime), nil
}

// RenderAdminSignupConfirmed builds the admin's confirmed signup
// notification.
func (s *Service) RenderAdminSignupConfirmed(ctx context.Context, a AdminSignupConfirmedMailArgs) (*mail.Envelope, error) {
	user, missing, err := gone(s.store.UserByID(ctx, a.UserID))
	if missing || err != nil {
		return nil, err
	}

	return mail.AdminSignupConfirmed(s.ident.Site, s.ident.From, s.ident.AdminAddress, user), nil
}

// RenderAdminPageFailure builds the admin's page failure report.
func (s *Service) RenderAdminPageFailure(ctx context.Context, a AdminPageFailureMailArgs) (*mail.Envelope, error) {
	var user *model.User

	if a.UserID != "" {
		u, missing, err := gone(s.store.UserByID(ctx, a.UserID))
		if missing || err != nil {
			return nil, err
		}

		user = u
	}

	return mail.AdminPageFailure(s.ident.Site, s.ident.From, s.ident.AdminAddress, a.Report, user), nil
}
