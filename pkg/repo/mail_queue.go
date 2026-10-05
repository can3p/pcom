package repo

import (
	"context"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// MailQueue stores a mail on the caller's executor, so the mail goes out only
// if the transaction commits. It is not a delivery channel: delivery is gogo's
// sender.Sender, which the queue's poller calls later.
type MailQueue interface {
	Send(ctx context.Context, exec boil.ContextExecutor, uniqueID, emailType string, mail *sender.Mail) error
}

// SendMail hands a mail to the queue on the store's executor, so a service
// that sends inside Tx sends only if the transaction commits.
func (s *Store) SendMail(ctx context.Context, snd MailQueue, uniqueID, emailType string, mail *sender.Mail) error {
	return snd.Send(ctx, s.exec, uniqueID, emailType, mail)
}

// GetPendingEmails returns emails ready to be sent, locked for update.
func (s *Store) GetPendingEmails(ctx context.Context) ([]*model.OutgoingEmail, error) {
	return all[model.OutgoingEmail](core.OutgoingEmails(
		core.OutgoingEmailWhere.Status.EQ(core.OutgoingEmailStatusNew),
		core.OutgoingEmailWhere.TryAt.LT(time.Now()),
		qm.For("UPDATE SKIP LOCKED"),
	).All(ctx, s.exec))
}

// UpdateOutgoingEmail updates an outgoing email record.
func (s *Store) UpdateOutgoingEmail(ctx context.Context, email *model.OutgoingEmail) error {
	return write(email, func(c *core.OutgoingEmail) error {
		_, err := c.Update(ctx, s.exec, boil.Infer())
		return err
	})
}

// CreateOrUpdateOutgoingEmail creates or updates an outgoing email using upsert.
func (s *Store) CreateOrUpdateOutgoingEmail(ctx context.Context, email *model.OutgoingEmail) error {
	return write(email, func(c *core.OutgoingEmail) error {
		return c.Upsert(ctx, s.exec, false, []string{core.OutgoingEmailColumns.EmailType, core.OutgoingEmailColumns.UniqueID}, boil.Infer(), boil.Infer())
	})
}
