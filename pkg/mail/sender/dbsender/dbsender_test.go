package dbsender

import (
	"context"
	"errors"
	"net/mail"
	"sync"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// panicSender is a sender.Sender that always panics: it stands in for a real
// sender misbehaving badly, to pin that sendEmails recovers instead of
// crashing the poller.
type panicSender struct{}

func (panicSender) Send(ctx context.Context, exec boil.ContextExecutor, uniqueID string, emailType string, m *sender.Mail) error {
	panic("boom")
}

// newOutgoingEmail and trySend and listOutgoing wrap the setup this file
// repeats, so each test case reads as one line.
func newOutgoingEmail(t *testing.T, ctx context.Context, db *sqlx.DB) *core.OutgoingEmail {
	t.Helper()

	return testutil.Must(factory.OutgoingEmail(ctx, db, "welcome"))(t)
}

// trySend runs trySendEmail in its own committed transaction, the way the
// poller does, and fails the test immediately on any error.
func trySend(t *testing.T, ctx context.Context, m *dbSender, db *sqlx.DB, outgoing *core.OutgoingEmail) {
	t.Helper()

	tx := testutil.Must(db.Begin())(t)
	require.NoError(t, m.trySendEmail(ctx, tx, outgoing))
	require.NoError(t, tx.Commit())
}

func listOutgoing(t *testing.T, ctx context.Context, db *sqlx.DB, mods ...qm.QueryMod) core.OutgoingEmailSlice {
	t.Helper()

	return testutil.Must(factory.ListOutgoingEmails(ctx, db, mods...))(t)
}

func TestSend_IdempotentOnEmailTypeAndUniqueID(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	s := NewSender(db, fakesender.New())

	m := &sender.Mail{
		From:    mail.Address{Address: "from@example.test"},
		To:      []mail.Address{{Address: "to@example.test"}},
		Subject: "hi",
	}

	require.NoError(t, s.Send(ctx, db, "unique-1", "welcome", m))
	require.NoError(t, s.Send(ctx, db, "unique-1", "welcome", m))

	rows := listOutgoing(t, ctx, db, core.OutgoingEmailWhere.EmailType.EQ("welcome"))
	require.Len(t, rows, 1, "sending the same (email_type, unique_id) twice must not queue a second row")

	// A different unique_id for the same email_type is a distinct email.
	require.NoError(t, s.Send(ctx, db, "unique-2", "welcome", m))

	rows = listOutgoing(t, ctx, db, core.OutgoingEmailWhere.EmailType.EQ("welcome"))
	require.Len(t, rows, 2)
}

func TestTrySendEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("retry schedule", func(t *testing.T) {
		t.Parallel()

		real := fakesender.New()
		m := NewSender(db, real)

		outgoing := newOutgoingEmail(t, ctx, db)
		require.Equal(t, 0, outgoing.AttemptsNumber)

		real.FailWith(errors.New("smtp down"))

		// Attempts 1 through attemptsNumber fail and reschedule with the
		// matching interval, keeping the email in the New status.
		for i := range attemptsNumber {
			trySend(t, ctx, m, db, outgoing)

			require.Equal(t, i+1, outgoing.AttemptsNumber, "attempt %d", i+1)
			require.Equal(t, core.OutgoingEmailStatusNew, outgoing.Status, "attempt %d", i+1)
			require.WithinDuration(t, time.Now().Add(retryIntervals[i]), outgoing.TryAt, 5*time.Second, "attempt %d", i+1)
		}

		// One more failure past attemptsNumber gives up for good.
		trySend(t, ctx, m, db, outgoing)

		require.Equal(t, core.OutgoingEmailStatusFailed, outgoing.Status)
		require.Equal(t, attemptsNumber, outgoing.AttemptsNumber)
		require.Empty(t, real.Sent(), "the real sender never succeeded, so nothing should be recorded as sent")
	})

	t.Run("marks sent on success", func(t *testing.T) {
		t.Parallel()

		real := fakesender.New()
		m := NewSender(db, real)

		outgoing := newOutgoingEmail(t, ctx, db)

		trySend(t, ctx, m, db, outgoing)

		require.Equal(t, core.OutgoingEmailStatusSent, outgoing.Status)
		require.True(t, outgoing.SentAt.Valid)
		require.WithinDuration(t, time.Now(), outgoing.SentAt.Time, 5*time.Second)

		sent := real.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, outgoing.UniqueID, sent[0].UniqueID)
		require.Equal(t, "welcome", sent[0].EmailType)
	})

	t.Run("rejects undecodable payload", func(t *testing.T) {
		t.Parallel()

		// An email whose payload isn't valid JSON is rejected before anything
		// touches the database, so this needs neither a real db nor a tx.
		m := NewSender(nil, fakesender.New())
		outgoing := &core.OutgoingEmail{
			ID:      "not-persisted",
			Payload: []byte("not-json"),
		}

		require.Error(t, m.trySendEmail(ctx, nil, outgoing))
	})
}

// TestSendEmails's subtests share one database but run one at a time: unlike
// trySendEmail, sendEmails scans the whole outgoing_emails table, so two
// subtests racing in parallel could pick up each other's rows.
func TestSendEmails(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("processes pending and updates status", func(t *testing.T) {
		real := fakesender.New()
		m := NewSender(db, real)

		outgoing := newOutgoingEmail(t, ctx, db)

		require.NoError(t, m.sendEmails(ctx))

		rows := listOutgoing(t, ctx, db, core.OutgoingEmailWhere.ID.EQ(outgoing.ID))
		require.Len(t, rows, 1)
		require.Equal(t, core.OutgoingEmailStatusSent, rows[0].Status)
		require.Len(t, real.Sent(), 1)
	})

	t.Run("no pending emails is a noop", func(t *testing.T) {
		setup := fakesender.New()
		setupSender := NewSender(db, setup)

		// Already sent: must not be retried.
		sentEmail := newOutgoingEmail(t, ctx, db)
		trySend(t, ctx, setupSender, db, sentEmail)
		require.Equal(t, core.OutgoingEmailStatusSent, sentEmail.Status)

		// Exhausted its attempts: must not be retried either.
		failedEmail := newOutgoingEmail(t, ctx, db)
		setup.FailWith(errors.New("smtp down"))
		for range attemptsNumber + 1 {
			trySend(t, ctx, setupSender, db, failedEmail)
		}
		require.Equal(t, core.OutgoingEmailStatusFailed, failedEmail.Status)

		// A failed attempt reschedules the retry into the future: not due yet.
		futureEmail := newOutgoingEmail(t, ctx, db)
		trySend(t, ctx, setupSender, db, futureEmail)
		require.Equal(t, core.OutgoingEmailStatusNew, futureEmail.Status)
		require.True(t, futureEmail.TryAt.After(time.Now()), "a failed attempt should reschedule the email into the future")
		setup.FailWith(nil)

		real := fakesender.New()
		m := NewSender(db, real)

		require.NoError(t, m.sendEmails(ctx))

		require.Empty(t, real.Sent(), "sendEmails must not touch sent, failed or not-yet-due rows")

		rows := listOutgoing(t, ctx, db)
		byID := map[string]*core.OutgoingEmail{}
		for _, r := range rows {
			byID[r.ID] = r
		}
		require.Equal(t, core.OutgoingEmailStatusSent, byID[sentEmail.ID].Status, "a sent email's status must not change")
		require.Equal(t, core.OutgoingEmailStatusFailed, byID[failedEmail.ID].Status, "a failed email's status must not change")
		require.Equal(t, core.OutgoingEmailStatusNew, byID[futureEmail.ID].Status, "a not-yet-due email's status must not change")
	})

	t.Run("recovers from real sender panic", func(t *testing.T) {
		m := NewSender(db, panicSender{})

		newOutgoingEmail(t, ctx, db)

		err := m.sendEmails(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "sendEmails panicked")
	})

	t.Run("concurrent runs send once", func(t *testing.T) {
		// own database: earlier subtests leave pending rows behind, which the
		// second run would legitimately claim.
		db := testdb.New(t).DB
		real := newBlockingSender()
		m := NewSender(db, real)

		newOutgoingEmail(t, ctx, db)

		// Two sendEmails runs race over the same pending row. Correct behavior
		// is that the SELECT ... FOR UPDATE SKIP LOCKED runs inside the same
		// transaction that later updates the row, so only one of the two racing
		// runs can claim it and the real sender is invoked exactly once. The
		// blockingSender forces both runs to actually reach the real sender
		// before either completes, so the double send happens deterministically
		// while the bug exists instead of only when the runs happen to race.
		const runners = 2
		start := make(chan struct{})
		errs := make(chan error, runners)

		var wg sync.WaitGroup
		for range runners {
			wg.Go(func() {
				<-start
				errs <- m.sendEmails(ctx)
			})
		}

		close(start)
		wg.Wait()
		close(errs)

		for err := range errs {
			require.NoError(t, err)
		}

		require.Len(t, real.Sent(), 1, "the locked row must be claimed by exactly one concurrent sendEmails run")
	})
}

// blockingSender is a sender.Sender that makes the first of two concurrent
// Send calls wait for the second one (or a short timeout) before either
// returns. It forces both racing sendEmails runs to actually reach the real
// sender for the same row while bug #113 is present, instead of depending on
// which run happens to finish first: without it, the race is
// timing-dependent and the test can pass even though the bug is still there.
type blockingSender struct {
	mu      sync.Mutex
	sent    []string
	arrived int
	release chan struct{}
	once    sync.Once
}

func newBlockingSender() *blockingSender {
	return &blockingSender{release: make(chan struct{})}
}

func (s *blockingSender) Send(ctx context.Context, exec boil.ContextExecutor, uniqueID string, emailType string, m *sender.Mail) error {
	s.mu.Lock()
	s.arrived++
	n := s.arrived
	s.mu.Unlock()

	if n >= 2 {
		s.once.Do(func() { close(s.release) })
	} else {
		select {
		case <-s.release:
		case <-time.After(2 * time.Second):
			// Only one caller ever arrived (e.g. because #113 got fixed and
			// the second run's row got skip-locked): don't hang the test.
			s.once.Do(func() { close(s.release) })
		}
	}

	s.mu.Lock()
	s.sent = append(s.sent, uniqueID)
	s.mu.Unlock()

	return nil
}

func (s *blockingSender) Sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.sent))
	copy(out, s.sent)

	return out
}

func TestRunPoller_SendsPendingAndStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx, cancel := context.WithCancel(context.Background())

	real := fakesender.New()
	m := NewSender(db, real)

	newOutgoingEmail(t, context.Background(), db)

	done := make(chan struct{})
	go func() {
		m.RunPoller(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return len(real.Sent()) == 1
	}, pollEvery+5*time.Second, 200*time.Millisecond, "RunPoller should have sent the pending email on its first tick")

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPoller did not stop after its context was cancelled")
	}
}
