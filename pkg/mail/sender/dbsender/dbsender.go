package dbsender

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
)

// DefaultRetryIntervals is the wait before each retry of a failed mail, equal
// to the default of config.Mail.RetryIntervals. A mail fails for good after
// one attempt more than there are intervals.
var DefaultRetryIntervals = []time.Duration{10 * time.Second, 60 * time.Second, 30 * time.Minute}

type dbSender struct {
	realSender     sender.Sender
	store          *repo.Store
	retryIntervals []time.Duration
}

// Option configures the sender.
type Option func(*dbSender)

// WithRetryIntervals sets the wait before each retry of a failed mail; a mail
// fails for good after len(intervals)+1 attempts.
func WithRetryIntervals(intervals []time.Duration) Option {
	return func(m *dbSender) {
		m.retryIntervals = slices.Clone(intervals)
	}
}

// compile-time check that dbSender is the mail queue.
var _ repo.MailQueue = (*dbSender)(nil)

func NewSender(db *repo.Store, realSender sender.Sender, opts ...Option) *dbSender {
	m := &dbSender{
		realSender:     realSender,
		store:          db,
		retryIntervals: slices.Clone(DefaultRetryIntervals),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// RunPoller sends the queued mail every interval until ctx is done.
func (m *dbSender) RunPoller(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)

	for {
		select {
		case <-ticker.C:
			if err := m.sendEmails(ctx); err != nil {
				slog.Warn("Failed to send emails", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *dbSender) sendEmails(ctx context.Context) (err error) {
	// we don't want any code including the real sender to crash
	// the scheduler
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("sendEmails panicked: %v", panicErr)
		}
	}()

	ids, err := m.store.ListDueEmailIDs(ctx)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := m.sendOne(ctx, id); err != nil {
			slog.Warn("failed to send email", "email_id", id, "err", err)
		}
	}

	return nil
}

// sendOne sends one mail in its own transaction: it locks the row, sends the
// mail and records the outcome. A row another worker holds, or that is no
// longer new, is skipped. A failure here leaves every other mail alone.
func (m *dbSender) sendOne(ctx context.Context, id string) error {
	return m.store.Tx(ctx, func(tx *repo.Store) error {
		outgoing, err := tx.LockDueEmail(ctx, id)
		if err != nil {
			return err
		}

		if outgoing == nil {
			return nil
		}

		return m.trySendEmail(ctx, tx, outgoing)
	})
}

func (m *dbSender) trySendEmail(ctx context.Context, tx *repo.Store, outgoing *model.OutgoingEmail) error {
	var payload sender.Mail

	if err := json.Unmarshal(outgoing.Payload, &payload); err != nil {
		return err
	}

	slog.Debug("Trying to send an email for real", "id", outgoing.ID, "to", payload.To)
	sendErr := m.realSender.Send(ctx, &payload)

	if sendErr == nil {
		outgoing.Status = model.OutgoingEmailStatusSent
		outgoing.SentAt = new(time.Now())
	} else {
		if outgoing.AttemptsNumber < len(m.retryIntervals) {
			outgoing.TryAt = time.Now().Add(m.retryIntervals[outgoing.AttemptsNumber])
			outgoing.AttemptsNumber = outgoing.AttemptsNumber + 1
		} else {
			outgoing.Status = model.OutgoingEmailStatusFailed
		}
	}

	return tx.UpdateOutgoingEmail(ctx, outgoing)
}

// Send schedules an email for sending. Email with duplicate (emailType, uniqueID) tuple will be skipped
func (m *dbSender) Send(ctx context.Context, exec repo.Executor, uniqueID string, emailType string, mail *sender.Mail) error {
	id, err := uuid.NewV7()

	if err != nil {
		return err
	}

	uniqueUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(uniqueID))

	b, err := json.Marshal(mail)

	if err != nil {
		return err
	}

	outgoing := model.OutgoingEmail{
		ID:        id.String(),
		UniqueID:  uniqueUUID.String(),
		Payload:   b,
		Status:    model.OutgoingEmailStatusNew,
		TryAt:     time.Now(),
		EmailType: emailType,
	}

	slog.Debug("Scheduling email", "uniqueID", uniqueID, "email_type", emailType, "to", mail.To)

	// this action is really dumb in a sense that we only attempt to put an email into the queue and bail if it's already there
	// We use repo.Using to wrap the legacy code, as this is the executor path from services
	return repo.Using(exec).CreateOrUpdateOutgoingEmail(ctx, &outgoing)
}
