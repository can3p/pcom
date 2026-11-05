package jobs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/jobs"
	pmail "github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

const testMailType = "test_mail"

type mailArgs struct {
	Seq int    `json:"seq"`
	UID string `json:"uid"`
}

func (mailArgs) Kind() string { return "mail." + testMailType }

func testMail(subject string) *sender.Mail {
	return &sender.Mail{
		From:    mail.Address{Address: "from@example.com"},
		To:      []mail.Address{{Address: "to@example.com"}},
		Subject: subject,
		Text:    "body",
	}
}

func insertJob(t *testing.T, db *sql.DB, args interface{ Kind() string }) {
	t.Helper()

	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)
	require.NoError(t, ins.Insert(context.Background(), db, args))
}

func sentRows(t *testing.T, db *sql.DB) []*model.SentEmail {
	t.Helper()

	rows, err := factory.ListSentEmails(context.Background(), db, testMailType)
	require.NoError(t, err)

	return rows
}

func TestRegisterMail_DeliversOnceAndRecords(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	fake := fakesender.New()
	reg := newRegistry(db, fake.Delivery())

	jobs.RegisterMail(reg, func(_ context.Context, a mailArgs) (*pmail.Envelope, error) {
		return &pmail.Envelope{UniqueID: a.UID, Type: testMailType, Mail: testMail("hello")}, nil
	})

	insertJob(t, db, mailArgs{Seq: 1, UID: "uid-1"})
	insertJob(t, db, mailArgs{Seq: 2, UID: "uid-1"})
	insertJob(t, db, mailArgs{Seq: 3, UID: "uid-other"})

	startRunner(t, db, reg)
	requireStates(t, db, "mail."+testMailType, "completed", "completed", "completed")

	sent := fake.Sent()
	require.Len(t, sent, 2)

	for _, s := range sent {
		require.Equal(t, "hello", s.Mail.Subject)
	}

	rows := sentRows(t, db)
	require.Len(t, rows, 2)

	ids := []string{rows[0].UniqueID, rows[1].UniqueID}
	require.ElementsMatch(t, []string{jobs.UniqueKey("uid-1"), jobs.UniqueKey("uid-other")}, ids)

	for _, r := range rows {
		require.Equal(t, testMailType, r.EmailType)

		var m sender.Mail

		require.NoError(t, json.Unmarshal(r.Payload, &m))
		require.Equal(t, "hello", m.Subject)
		require.WithinDuration(t, time.Now(), r.SentAt, time.Minute)
	}
}

func TestRegisterMail_NilEnvelopeSendsNothing(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	fake := fakesender.New()
	reg := newRegistry(db, fake.Delivery())

	var rendered atomic.Int64

	jobs.RegisterMail(reg, func(_ context.Context, _ mailArgs) (*pmail.Envelope, error) {
		rendered.Add(1)

		return nil, nil //nolint:nilnil // nothing to send any more
	})

	insertJob(t, db, mailArgs{Seq: 1, UID: "gone"})

	startRunner(t, db, reg)
	requireStates(t, db, "mail."+testMailType, "completed")

	require.EqualValues(t, 1, rendered.Load())
	require.Empty(t, fake.Sent())
	require.Empty(t, sentRows(t, db))
}

func TestRegisterMail_RenderErrorRetriesThenCancels(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	fake := fakesender.New()
	reg := newRegistry(db, fake.Delivery(), jobs.WithRetryIntervals([]time.Duration{time.Millisecond}))

	var rendered atomic.Int64

	jobs.RegisterMail(reg, func(_ context.Context, _ mailArgs) (*pmail.Envelope, error) {
		rendered.Add(1)

		return nil, errors.New("no render")
	})

	insertJob(t, db, mailArgs{Seq: 1, UID: "x"})

	startRunner(t, db, reg)
	requireStates(t, db, "mail."+testMailType, "cancelled")

	require.EqualValues(t, 2, rendered.Load())
	require.Empty(t, fake.Sent())
}

func TestRegisterMail_DeliveryFailure(t *testing.T) {
	t.Parallel()

	t.Run("always failing: two attempts, cancelled, nothing recorded", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		fake := fakesender.New()
		fake.FailWith(errors.New("smtp down"))

		reg := newRegistry(db, fake.Delivery(), jobs.WithRetryIntervals([]time.Duration{time.Millisecond}))

		var rendered atomic.Int64

		jobs.RegisterMail(reg, func(_ context.Context, a mailArgs) (*pmail.Envelope, error) {
			rendered.Add(1)

			return &pmail.Envelope{UniqueID: a.UID, Type: testMailType, Mail: testMail("hi")}, nil
		})

		insertJob(t, db, mailArgs{Seq: 1, UID: "fail"})

		startRunner(t, db, reg)
		requireStates(t, db, "mail."+testMailType, "cancelled")

		require.EqualValues(t, 2, rendered.Load())
		require.Empty(t, fake.Sent())
		require.Empty(t, sentRows(t, db))
	})

	t.Run("failing once: retried, one row, one delivery", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		fake := fakesender.New()

		var calls atomic.Int64

		flaky := flakyDelivery{inner: fake.Delivery(), failFirst: 1, calls: &calls}
		reg := newRegistry(db, flaky, jobs.WithRetryIntervals([]time.Duration{time.Millisecond}))

		jobs.RegisterMail(reg, func(_ context.Context, a mailArgs) (*pmail.Envelope, error) {
			return &pmail.Envelope{UniqueID: a.UID, Type: testMailType, Mail: testMail("hi")}, nil
		})

		insertJob(t, db, mailArgs{Seq: 1, UID: "flaky"})

		startRunner(t, db, reg)
		requireStates(t, db, "mail."+testMailType, "completed")

		require.EqualValues(t, 2, calls.Load())
		require.Len(t, fake.Sent(), 1)

		rows := sentRows(t, db)
		require.Len(t, rows, 1)
		require.Equal(t, jobs.UniqueKey("flaky"), rows[0].UniqueID)
	})
}

type flakyDelivery struct {
	inner     sender.Sender
	failFirst int64
	calls     *atomic.Int64
}

func (f flakyDelivery) Send(ctx context.Context, m *sender.Mail) error {
	if f.calls.Add(1) <= f.failFirst {
		return errors.New("transient failure")
	}

	return f.inner.Send(ctx, m)
}
