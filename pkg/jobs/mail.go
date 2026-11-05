package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// RegisterMail registers a mail kind A. Its worker calls render, which loads
// what the mail shows as it is now and builds it with a pkg/mail
// constructor, then sends the envelope: it records the mail in sent_emails
// and hands it to the delivery in one transaction. A mail already recorded
// under the same type and unique id is not sent again. render returns a nil
// envelope when there is nothing to send any more (a deleted post, a
// recipient gone), and the job completes.
//
// A failed attempt (render or delivery) is retried after the next of the
// registry's retry intervals; after one attempt more than there are
// intervals the mail fails for good.
func RegisterMail[A river.JobArgs](r *Registry, render func(ctx context.Context, args A) (*mail.Envelope, error)) {
	river.AddWorker(r.workers, &mailWorker[A]{render: render, deliverer: r.deliverer()})
}

// RenderedMailArgs is a mail rendered before the job queue existed: the
// unsent rows of the old outgoing_emails table, which
// 20261105121000-sent_emails.sql and 20261105130000-drop_outgoing_emails.sql
// turned into jobs. UniqueKey is already the SHA-1 uuid.
type RenderedMailArgs struct {
	UniqueKey string      `json:"unique_key"`
	EmailType string      `json:"email_type"`
	Mail      sender.Mail `json:"mail"`
}

// Kind names the job kind.
func (RenderedMailArgs) Kind() string { return "mail.rendered" }

// deliverer sends one mail and records it.
type deliverer struct {
	store          *repo.Store
	delivery       sender.Sender
	retryIntervals []time.Duration
}

func (r *Registry) deliverer() deliverer {
	return deliverer{store: r.store, delivery: r.delivery, retryIntervals: r.retryIntervals}
}

// UniqueKey is the key sent_emails files a mail's unique id string under.
func UniqueKey(uniqueID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(uniqueID)).String()
}

// send records the mail and delivers it in one transaction: a delivery
// error rolls the record back, and a mail already recorded is skipped.
func (d deliverer) send(ctx context.Context, emailType, uniqueKey string, m *sender.Mail) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	return d.store.Tx(ctx, func(tx *repo.Store) error {
		fresh, err := tx.RecordSentEmail(ctx, &model.SentEmail{
			ID:        id.String(),
			UniqueID:  uniqueKey,
			EmailType: emailType,
			Payload:   payload,
			SentAt:    time.Now().UTC(),
		})
		if err != nil || !fresh {
			return err
		}

		if err := d.delivery.Send(ctx, m); err != nil {
			return fmt.Errorf("delivering %s mail: %w", emailType, err)
		}

		return nil
	})
}

// failed turns the error of attempt into the job's outcome: nil stays nil,
// the last allowed attempt cancels the job for good, anything else retries.
func (d deliverer) failed(attempt int, err error) error {
	if err != nil && attempt > len(d.retryIntervals) {
		return river.JobCancel(err)
	}

	return err
}

// nextRetry is when attempt's retry runs.
func (d deliverer) nextRetry(attempt int) time.Time {
	if len(d.retryIntervals) == 0 {
		return time.Now()
	}

	i := min(max(attempt-1, 0), len(d.retryIntervals)-1)

	return time.Now().Add(d.retryIntervals[i])
}

type mailWorker[A river.JobArgs] struct {
	river.WorkerDefaults[A]
	deliverer
	render func(ctx context.Context, args A) (*mail.Envelope, error)
}

func (w *mailWorker[A]) Work(ctx context.Context, job *river.Job[A]) error {
	env, err := w.render(ctx, job.Args)
	if err != nil {
		return w.failed(job.Attempt, fmt.Errorf("rendering %s: %w", job.Kind, err))
	}

	if env == nil {
		return nil
	}

	return w.failed(job.Attempt, w.send(ctx, env.Type, UniqueKey(env.UniqueID), env.Mail))
}

func (w *mailWorker[A]) NextRetry(job *river.Job[A]) time.Time {
	return w.nextRetry(job.Attempt)
}

type renderedMailWorker struct {
	river.WorkerDefaults[RenderedMailArgs]
	deliverer
}

func (w *renderedMailWorker) Work(ctx context.Context, job *river.Job[RenderedMailArgs]) error {
	return w.failed(job.Attempt, w.send(ctx, job.Args.EmailType, job.Args.UniqueKey, &job.Args.Mail))
}

func (w *renderedMailWorker) NextRetry(job *river.Job[RenderedMailArgs]) time.Time {
	return w.nextRetry(job.Attempt)
}
