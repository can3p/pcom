package mail

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"net/mail"
	"os"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/pkg/errors"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

func SendInvite(ctx context.Context, db boil.ContextExecutor, sender sender.Sender, senderUser *core.User, to string) error {
	to = pgsession.NormalizeEmail(to)

	exists := core.Users(
		pgsession.EmailIs(to),
	).ExistsP(ctx, db)

	if exists {
		return errors.Errorf("user with email address [%s] already exists", to)
	}

	if PendingInvitationExists(ctx, db, to) {
		return errors.Errorf("an invitation to [%s] is already pending", to)
	}

	invite, err := core.UserInvitations(
		core.UserInvitationWhere.UserID.EQ(senderUser.ID),
		core.UserInvitationWhere.InvitationEmail.IsNull(),
		qm.For("update skip locked"),
	).One(ctx, db)

	if err == sql.ErrNoRows {
		return errors.Errorf("user [%s] does not have any unused invites", senderUser.Email)
	}

	if err != nil {
		return errors.Errorf("Failed to lock the invite: %v", err)
	}

	invite.InvitationEmail = null.StringFrom(to)
	invite.InvitationSentAt = null.TimeFrom(time.Now())

	if _, err := invite.Update(ctx, db, boil.Infer()); err != nil {
		return errors.Errorf("Failed to save the invite: %v", err)
	}

	return sendActualInvitation(ctx, db, sender, invite, senderUser, to)
}

func sendActualInvitation(ctx context.Context, exec boil.ContextExecutor, s sender.Sender, invite *core.UserInvitation, user *core.User, to string) error {
	link := links.AbsLink("invite", invite.ID)

	mail := &sender.Mail{
		From: mail.Address{
			Address: os.Getenv("SENDER_ADDRESS"),
			Name:    "Your pcom",
		},
		To: []mail.Address{
			{
				Address: to,
			},
		},
		Subject: "Welcome to pcom",
		Text: fmt.Sprintf(`
	Hi!

	Welcome to pcom! Please follow the link to set up your account.

	%s`, link),
		Html: fmt.Sprintf(`
	<p>Hi!</p>

	<p>Welcome to pcom! Please follow the link to set up your account.</p>

	<a href="%s">%s</a>`, html.EscapeString(link), html.EscapeString(link)),
	}

	err := s.Send(ctx, exec, invite.ID, "user_invitation", mail)

	if err != nil {
		return errors.Wrap(err, "failed to queue email")
	}

	return nil
}

// PendingInvitationExists reports whether an invitation to the address has
// been sent and not yet used to create an account.
func PendingInvitationExists(ctx context.Context, db boil.ContextExecutor, email string) bool {
	return core.UserInvitations(
		qm.Where("lower(btrim(invitation_email)) = ?", pgsession.NormalizeEmail(email)),
		core.UserInvitationWhere.CreatedUserID.IsNull(),
	).ExistsP(ctx, db)
}
