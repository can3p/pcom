package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

// Invitation returns an invitation nobody has accepted yet, with its inviter
// loaded (invite.R.User).
func (s *Service) Invitation(ctx context.Context, id string) (*core.UserInvitation, error) {
	return notFound(s.store.OpenInvitationByID(ctx, id))
}

// CheckInviteEmail says whether the actor may invite the address. It returns
// a ValidationError with the reason when they may not.
func (s *Service) CheckInviteEmail(ctx context.Context, email string) error {
	if err := mail.ValidateFormat(email); err != nil {
		return service.Invalid("email", err.Error())
	}

	if exists, err := s.store.UserEmailExists(ctx, pgsession.NormalizeEmail(email)); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "email is already registered in the system")
	}

	if exists, err := s.store.PendingInvitationExists(ctx, pgsession.NormalizeEmail(email)); err != nil {
		return err
	} else if exists {
		return service.Invalid("email", "an invitation to this email is already pending")
	}

	return nil
}

// SendInvite spends one of the actor's invitations on the address and mails
// the invitation link.
func (s *Service) SendInvite(ctx context.Context, actor *core.User, to string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	to = pgsession.NormalizeEmail(to)

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if exists, err := tx.UserEmailExists(ctx, to); err != nil {
			return err
		} else if exists {
			return service.Invalid("email", fmt.Sprintf("user with email address [%s] already exists", to))
		}

		if exists, err := tx.PendingInvitationExists(ctx, to); err != nil {
			return err
		} else if exists {
			return service.Invalid("email", fmt.Sprintf("an invitation to [%s] is already pending", to))
		}

		invite, err := tx.LockUnusedInvitation(ctx, actor.ID)
		if err == repo.ErrNotFound {
			return service.Invalid("", fmt.Sprintf("user [%s] does not have any unused invites", actor.Email))
		} else if err != nil {
			return fmt.Errorf("failed to lock the invite: %w", err)
		}

		invite.InvitationEmail = null.StringFrom(to)
		invite.InvitationSentAt = null.TimeFrom(time.Now())

		if err := tx.SaveInvitation(ctx, invite); err != nil {
			return fmt.Errorf("failed to save the invite: %w", err)
		}

		return s.send(ctx, tx, mail.Invitation(s.ident.Site, s.ident.From, invite, to))
	})
}

// AcceptInvite creates the account an invitation was sent for, connects it to
// the inviter and gives it one invitation of its own to make things (slowly)
// spread. The account is confirmed, since the invitation reached its email.
func (s *Service) AcceptInvite(ctx context.Context, invite *core.UserInvitation, username, password string) (*core.User, error) {
	if password == "" || username == "" {
		return nil, service.Invalid("", "Not enough data")
	}

	u := &core.User{
		ID:                uuid.NewString(),
		Email:             pgsession.NormalizeEmail(invite.InvitationEmail.String),
		Username:          username,
		Pwdhash:           null.StringFrom(pgsession.HashPassword(password)),
		EmailConfirmedAt:  null.TimeFrom(time.Now()),
		SignupAttribution: null.StringFrom("accepted_invite"),
	}

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.InsertUser(ctx, u); err != nil {
			return err
		}

		invite.CreatedUserID = null.StringFrom(u.ID)

		if err := tx.SaveInvitation(ctx, invite); err != nil {
			return err
		}

		if _, _, err := tx.CreateConnection(ctx, invite.UserID, u.ID); err != nil {
			return err
		}

		if err := s.send(ctx, tx, admin.NewUser(s.ident.Site, s.ident.From, s.ident.AdminAddress, u)); err != nil {
			return err
		}

		return tx.InsertInvitation(ctx, &core.UserInvitation{ID: uuid.NewString(), UserID: u.ID})
	})
	if err != nil {
		return nil, err
	}

	return u, nil
}
