package repo

import (
	"context"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// SendMail hands a mail to the sender on the store's executor. With the
// queueing sender that is an insert, so a service that sends inside Tx sends
// only if the transaction commits.
func (s *Store) SendMail(ctx context.Context, snd sender.Sender, uniqueID, emailType string, mail *sender.Mail) error {
	return snd.Send(ctx, s.exec, uniqueID, emailType, mail)
}

// GetPendingEmails returns emails ready to be sent, locked for update.
func (s *Store) GetPendingEmails(ctx context.Context) ([]*core.OutgoingEmail, error) {
	return core.OutgoingEmails(
		core.OutgoingEmailWhere.Status.EQ(core.OutgoingEmailStatusNew),
		core.OutgoingEmailWhere.TryAt.LT(time.Now()),
		qm.For("UPDATE SKIP LOCKED"),
	).All(ctx, s.exec)
}

// UpdateOutgoingEmail updates an outgoing email record.
func (s *Store) UpdateOutgoingEmail(ctx context.Context, email *core.OutgoingEmail) error {
	_, err := email.Update(ctx, s.exec, boil.Infer())
	return err
}

// CreateOrUpdateOutgoingEmail creates or updates an outgoing email using upsert.
func (s *Store) CreateOrUpdateOutgoingEmail(ctx context.Context, email *core.OutgoingEmail) error {
	return email.Upsert(ctx, s.exec, false, []string{core.OutgoingEmailColumns.EmailType, core.OutgoingEmailColumns.UniqueID}, boil.Infer(), boil.Infer())
}
