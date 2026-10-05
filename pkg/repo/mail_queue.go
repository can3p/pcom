package repo

import (
	"context"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model"
)

// MailQueue stores a mail on the caller's executor, so the mail goes out only
// if the transaction commits. It is not a delivery channel: delivery is gogo's
// sender.Sender, which the queue's poller calls later.
type MailQueue interface {
	Send(ctx context.Context, exec Executor, uniqueID, emailType string, mail *sender.Mail) error
}

// SendMail hands a mail to the queue on the store's executor, so a service
// that sends inside Tx sends only if the transaction commits.
func (s *Store) SendMail(ctx context.Context, snd MailQueue, uniqueID, emailType string, mail *sender.Mail) error {
	return snd.Send(ctx, s.exec, uniqueID, emailType, mail)
}

// GetPendingEmails returns emails ready to be sent, locked for update.
func (s *Store) GetPendingEmails(ctx context.Context) ([]*model.OutgoingEmail, error) {
	var emails []*model.OutgoingEmail

	err := s.query().NewSelect().Model(&emails).
		Where("status = ?", model.OutgoingEmailStatusNew).
		Where("try_at < ?", time.Now()).
		For("UPDATE SKIP LOCKED").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return emails, nil
}

// UpdateOutgoingEmail updates an outgoing email record.
func (s *Store) UpdateOutgoingEmail(ctx context.Context, email *model.OutgoingEmail) error {
	_, err := s.query().NewUpdate().Model(email).WherePK().Exec(ctx)

	return err
}

// CreateOrUpdateOutgoingEmail creates an outgoing email, skipping it when one
// with the same (email_type, unique_id) is already queued.
func (s *Store) CreateOrUpdateOutgoingEmail(ctx context.Context, email *model.OutgoingEmail) error {
	_, err := s.query().NewInsert().Model(email).
		On("CONFLICT (email_type, unique_id) DO NOTHING").
		Exec(ctx)

	return err
}
