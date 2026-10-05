package dbsender

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"sync"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// panicSender is a sender.Sender that always panics: it stands in for a real
// sender misbehaving badly, to pin that sendEmails recovers instead of
// crashing the poller.
type panicSender struct{}

func (panicSender) Send(ctx context.Context, m *sender.Mail) error {
	panic("boom")
}

// newOutgoingEmail and trySend and listOutgoing wrap the setup this file
// repeats, so each test case reads as one line.
func newOutgoingEmail(t *testing.T, ctx context.Context, db *repo.Store) *model.OutgoingEmail {
	t.Helper()

	return testutil.Must(factory.OutgoingEmail(ctx, db.Exec(), "welcome"))(t)
}

// trySend runs trySendEmail in its own committed transaction, the way the
// poller does, and fails the test immediately on any error.
func trySend(t *testing.T, ctx context.Context, m *dbSender, store *repo.Store, outgoing *model.OutgoingEmail) {
	t.Helper()

	require.NoError(t, store.Tx(ctx, func(tx *repo.Store) error {
		return m.trySendEmail(ctx, tx, outgoing)
	}))
}

func listOutgoing(t *testing.T, ctx context.Context, store *repo.Store, filter ...factory.OutgoingEmailFilter) []*model.OutgoingEmail {
	t.Helper()

	return testutil.Must(factory.ListOutgoingEmails(ctx, store.Exec(), filter...))(t)
}

func TestSend_IdempotentOnEmailTypeAndUniqueID(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	s := NewSender(store, fakesender.New().Delivery())

	m := &sender.Mail{
		From:    mail.Address{Address: "from@example.test"},
		To:      []mail.Address{{Address: "to@example.test"}},
		Subject: "hi",
	}

	require.NoError(t, s.Send(ctx, store.Exec(), "unique-1", "welcome", m))
	require.NoError(t, s.Send(ctx, store.Exec(), "unique-1", "welcome", m))

	rows := listOutgoing(t, ctx, store, factory.EmailType("welcome"))
	require.Len(t, rows, 1, "sending the same (email_type, unique_id) twice must not queue a second row")

	// A different unique_id for the same email_type is a distinct email.
	require.NoError(t, s.Send(ctx, store.Exec(), "unique-2", "welcome", m))

	rows = listOutgoing(t, ctx, store, factory.EmailType("welcome"))
	require.Len(t, rows, 2)
}

func TestTrySendEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	t.Run("retry schedule", func(t *testing.T) {
		t.Parallel()

		real := fakesender.New()
		m := NewSender(store, real.Delivery())

		outgoing := newOutgoingEmail(t, ctx, store)
		require.Equal(t, 0, outgoing.AttemptsNumber)

		real.FailWith(errors.New("smtp down"))

		// Attempts 1 through attemptsNumber fail and reschedule with the
		// matching interval, keeping the email in the New status.
		for i := range attemptsNumber {
			trySend(t, ctx, m, store, outgoing)

			require.Equal(t, i+1, outgoing.AttemptsNumber, "attempt %d", i+1)
			require.Equal(t, model.OutgoingEmailStatusNew, outgoing.Status, "attempt %d", i+1)
			require.WithinDuration(t, time.Now().Add(retryIntervals[i]), outgoing.TryAt, 5*time.Second, "attempt %d", i+1)
		}

		// One more failure past attemptsNumber gives up for good.
		trySend(t, ctx, m, store, outgoing)

		require.Equal(t, model.OutgoingEmailStatusFailed, outgoing.Status)
		require.Equal(t, attemptsNumber, outgoing.AttemptsNumber)
		require.Empty(t, real.Sent(), "the real sender never succeeded, so nothing should be recorded as sent")
	})

	t.Run("marks sent on success", func(t *testing.T) {
		t.Parallel()

		real := fakesender.New()
		m := NewSender(store, real.Delivery())

		outgoing := newOutgoingEmail(t, ctx, store)

		trySend(t, ctx, m, store, outgoing)

		require.Equal(t, model.OutgoingEmailStatusSent, outgoing.Status)
		require.NotNil(t, outgoing.SentAt)
		require.WithinDuration(t, time.Now(), *outgoing.SentAt, 5*time.Second)

		sent := real.Sent()
		require.Len(t, sent, 1)
		var queued sender.Mail
		require.NoError(t, json.Unmarshal(outgoing.Payload, &queued))
		require.Equal(t, &queued, sent[0].Mail, "the queued payload is what gets delivered")
	})

	t.Run("rejects undecodable payload", func(t *testing.T) {
		t.Parallel()

		// An email whose payload isn't valid JSON is rejected before anything
		// touches the database.
		m := NewSender(store, fakesender.New().Delivery())
		outgoing := &model.OutgoingEmail{
			ID:      "not-persisted",
			Payload: []byte("not-json"),
		}

		require.Error(t, m.trySendEmail(ctx, store, outgoing))
	})
}

// TestSendEmails's subtests share one database but run one at a time: unlike
// trySendEmail, sendEmails scans the whole outgoing_emails table, so two
// subtests racing in parallel could pick up each other's rows.
func TestSendEmails(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx := context.Background()

	t.Run("processes pending and updates status", func(t *testing.T) {
		real := fakesender.New()
		m := NewSender(store, real.Delivery())

		outgoing := newOutgoingEmail(t, ctx, store)

		require.NoError(t, m.sendEmails(ctx))

		rows := lo.Filter(listOutgoing(t, ctx, store), func(r *model.OutgoingEmail, _ int) bool { return r.ID == outgoing.ID })
		require.Len(t, rows, 1)
		require.Equal(t, model.OutgoingEmailStatusSent, rows[0].Status)
		require.Len(t, real.Sent(), 1)
	})

	t.Run("no pending emails is a noop", func(t *testing.T) {
		setup := fakesender.New()
		setupSender := NewSender(store, setup.Delivery())

		// Already sent: must not be retried.
		sentEmail := newOutgoingEmail(t, ctx, store)
		trySend(t, ctx, setupSender, store, sentEmail)
		require.Equal(t, model.OutgoingEmailStatusSent, sentEmail.Status)

		// Exhausted its attempts: must not be retried either.
		failedEmail := newOutgoingEmail(t, ctx, store)
		setup.FailWith(errors.New("smtp down"))
		for range attemptsNumber + 1 {
			trySend(t, ctx, setupSender, store, failedEmail)
		}
		require.Equal(t, model.OutgoingEmailStatusFailed, failedEmail.Status)

		// A failed attempt reschedules the retry into the future: not due yet.
		futureEmail := newOutgoingEmail(t, ctx, store)
		trySend(t, ctx, setupSender, store, futureEmail)
		require.Equal(t, model.OutgoingEmailStatusNew, futureEmail.Status)
		require.True(t, futureEmail.TryAt.After(time.Now()), "a failed attempt should reschedule the email into the future")
		setup.FailWith(nil)

		real := fakesender.New()
		m := NewSender(store, real.Delivery())

		require.NoError(t, m.sendEmails(ctx))

		require.Empty(t, real.Sent(), "sendEmails must not touch sent, failed or not-yet-due rows")

		rows := listOutgoing(t, ctx, store)
		byID := map[string]*model.OutgoingEmail{}
		for _, r := range rows {
			byID[r.ID] = r
		}
		require.Equal(t, model.OutgoingEmailStatusSent, byID[sentEmail.ID].Status, "a sent email's status must not change")
		require.Equal(t, model.OutgoingEmailStatusFailed, byID[failedEmail.ID].Status, "a failed email's status must not change")
		require.Equal(t, model.OutgoingEmailStatusNew, byID[futureEmail.ID].Status, "a not-yet-due email's status must not change")
	})

	t.Run("recovers from real sender panic", func(t *testing.T) {
		m := NewSender(store, panicSender{})

		newOutgoingEmail(t, ctx, store)

		err := m.sendEmails(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "sendEmails panicked")
	})

	t.Run("concurrent runs send once", func(t *testing.T) {
		// own database: earlier subtests leave pending rows behind, which the
		// second run would legitimately claim.
		db := testdb.New(t).DB
		store := repo.New(db)
		real := newBlockingSender()
		m := NewSender(store, real)

		newOutgoingEmail(t, ctx, store)

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

func (s *blockingSender) Send(ctx context.Context, m *sender.Mail) error {
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
	s.sent = append(s.sent, m.Subject)
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
	store := repo.New(db)
	ctx, cancel := context.WithCancel(context.Background())

	real := fakesender.New()
	m := NewSender(store, real.Delivery())

	newOutgoingEmail(t, context.Background(), store)

	done := make(chan struct{})
	go func() {
		m.RunPoller(ctx, 100*time.Millisecond)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return len(real.Sent()) == 1
	}, 2*time.Second, 50*time.Millisecond, "RunPoller should have sent the pending email on its first tick, after the interval it was given")

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPoller did not stop after its context was cancelled")
	}
}

// The interval is honoured: with a long one nothing is sent before it elapses.
func TestRunPoller_LongIntervalSendsNothingEarly(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := repo.New(db)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	real := fakesender.New()
	m := NewSender(store, real.Delivery())

	newOutgoingEmail(t, context.Background(), store)

	go m.RunPoller(ctx, time.Hour)

	require.Never(t, func() bool { return len(real.Sent()) > 0 }, 500*time.Millisecond, 50*time.Millisecond)
}
