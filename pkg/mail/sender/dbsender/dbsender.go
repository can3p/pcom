package dbsender

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/gogo/util/transact"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const pollEvery = 10 * time.Second
const attemptsNumber = 3

var retryIntervals = []time.Duration{10 * time.Second, 60 * time.Second, 30 * time.Minute}

type dbSender struct {
	realSender sender.Sender
	db         *sqlx.DB
}

func NewSender(db *sqlx.DB, realSender sender.Sender) *dbSender {
	return &dbSender{
		realSender: realSender,
		db:         db,
	}
}

func (m *dbSender) RunPoller(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)

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

	return transact.Transact(m.db, func(tx *sql.Tx) error {
		pending, err := core.OutgoingEmails(
			core.OutgoingEmailWhere.Status.EQ(core.OutgoingEmailStatusNew),
			core.OutgoingEmailWhere.TryAt.LT(time.Now()),
			qm.For("UPDATE SKIP LOCKED"),
		).All(ctx, m.db)

		if err != nil {
			return err
		}

		if len(pending) == 0 {
			return nil
		}

		for _, outgoing := range pending {
			if err := m.trySendEmail(ctx, tx, outgoing); err != nil {
				slog.Warn("failed to send email", "email_id", outgoing.ID, "err", err)
				continue
			}
		}

		return nil
	})
}

func (m *dbSender) trySendEmail(ctx context.Context, db *sql.Tx, outgoing *core.OutgoingEmail) error {
	var payload sender.Mail

	if err := outgoing.Payload.Unmarshal(&payload); err != nil {
		return err
	}

	slog.Debug("Trying to send an email for real", "id", outgoing.ID, "to", payload.To)
	sendErr := m.realSender.Send(ctx, db, outgoing.UniqueID, outgoing.EmailType, &payload)

	if sendErr == nil {
		outgoing.Status = core.OutgoingEmailStatusSent
		outgoing.SentAt = null.TimeFrom(time.Now())
	} else {
		if outgoing.AttemptsNumber < attemptsNumber {
			outgoing.TryAt = time.Now().Add(retryIntervals[outgoing.AttemptsNumber])
			outgoing.AttemptsNumber = outgoing.AttemptsNumber + 1
		} else {
			outgoing.Status = core.OutgoingEmailStatusFailed
		}
	}

	_, err := outgoing.Update(ctx, db, boil.Infer())

	return err
}

// Send schedules an email for sending. Email with duplicate (emailType, uniqueID) tuple will be skipped
func (m *dbSender) Send(ctx context.Context, exec boil.ContextExecutor, uniqueID string, emailType string, mail *sender.Mail) error {
	id, err := uuid.NewV7()

	if err != nil {
		return err
	}

	uniqueUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(uniqueID))

	b, err := json.Marshal(mail)

	if err != nil {
		return err
	}

	outgoing := core.OutgoingEmail{
		ID:        id.String(),
		UniqueID:  uniqueUUID.String(),
		Payload:   b,
		Status:    core.OutgoingEmailStatusNew,
		TryAt:     time.Now(),
		EmailType: emailType,
	}

	slog.Debug("Scheduling email", "uniqueID", uniqueID, "email_type", emailType, "to", mail.To)

	// this action is really dumb in a sense that we only attempt to put an email into the queue and bail if it's already there
	return outgoing.Upsert(ctx, exec, false, []string{core.OutgoingEmailColumns.EmailType, core.OutgoingEmailColumns.UniqueID}, boil.Infer(), boil.Infer())
}
