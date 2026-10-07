package repo

import (
	"context"
	"database/sql"
	"errors"
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

// ListDueEmailIDs returns the ids of the emails ready to be sent, oldest first.
// It takes no lock: LockDueEmail claims each one.
func (s *Store) ListDueEmailIDs(ctx context.Context) ([]string, error) {
	var ids []string

	err := s.query().NewSelect().Model((*model.OutgoingEmail)(nil)).
		Column("id").
		Where("status = ?", model.OutgoingEmailStatusNew).
		Where("try_at < ?", time.Now()).
		Order("id").
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// LockDueEmail locks one email for update inside the store's transaction. It
// returns nil when another worker holds the row or it is no longer new and due.
func (s *Store) LockDueEmail(ctx context.Context, id string) (*model.OutgoingEmail, error) {
	email := new(model.OutgoingEmail)

	err := s.query().NewSelect().Model(email).
		Where("id = ?", id).
		Where("status = ?", model.OutgoingEmailStatusNew).
		Where("try_at < ?", time.Now()).
		For("UPDATE SKIP LOCKED").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return email, nil
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
