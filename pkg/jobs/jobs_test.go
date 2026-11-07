package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/testutil/testlog"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

type pingArgs struct {
	N int `json:"n"`
}

func (pingArgs) Kind() string { return "test.ping" }

// startRunner starts a fast-polling runner over reg and stops it at cleanup.
func startRunner(t *testing.T, db *sql.DB, reg *jobs.Registry, opts ...jobs.Option) *jobs.Runner {
	t.Helper()

	runner, err := jobs.NewRunner(db, reg, append([]jobs.Option{jobs.WithPollInterval(10 * time.Millisecond)}, opts...)...)
	require.NoError(t, err)
	require.NoError(t, runner.Start(context.Background()))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = runner.Stop(ctx)
	})

	return runner
}

func newRegistry(db *sql.DB, d sender.Sender, opts ...jobs.RegistryOption) *jobs.Registry {
	return jobs.NewRegistry(repo.New(db), d, opts...)
}

// jobStates returns the state of every river_job of kind.
func jobStates(t *testing.T, db *sql.DB, kind string) []string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), `SELECT state::text FROM river_job WHERE kind = $1 ORDER BY id`, kind)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var states []string

	for rows.Next() {
		var s string

		require.NoError(t, rows.Scan(&s))
		states = append(states, s)
	}

	require.NoError(t, rows.Err())

	return states
}

func requireStates(t *testing.T, db *sql.DB, kind string, want ...string) {
	t.Helper()

	require.Eventually(t, func() bool {
		got := jobStates(t, db, kind)
		if len(got) != len(want) {
			return false
		}

		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}

		return true
	}, 10*time.Second, 10*time.Millisecond, "job states of %s, want %v, have %v", kind, want, jobStates(t, db, kind))
}

func TestInserter_Tx(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	var got atomic.Int64

	reg := newRegistry(db, fakesender.New().Delivery())
	jobs.Register(reg, func(_ context.Context, a pingArgs) error {
		got.Add(int64(a.N))

		return nil
	})

	boom := errors.New("rollback")

	err = repo.New(db).Tx(ctx, func(tx *repo.Store) error {
		require.NoError(t, tx.Enqueue(ctx, ins, pingArgs{N: 100}))

		return boom
	})
	require.ErrorIs(t, err, boom)
	require.Empty(t, jobStates(t, db, "test.ping"))

	require.NoError(t, repo.New(db).Tx(ctx, func(tx *repo.Store) error {
		return tx.Enqueue(ctx, ins, pingArgs{N: 7})
	}))
	require.Len(t, jobStates(t, db, "test.ping"), 1)

	startRunner(t, db, reg)
	requireStates(t, db, "test.ping", "completed")
	require.EqualValues(t, 7, got.Load())
}

type slowArgs struct{}

func (slowArgs) Kind() string { return "test.slow" }

func (slowArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: "slow"} }

// A job whose InsertOpts names a queue is worked only by a runner that has
// the queue.
func TestRunner_NamedQueue(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	reg := newRegistry(db, fakesender.New().Delivery())
	jobs.Register(reg, func(context.Context, pingArgs) error { return nil })
	jobs.Register(reg, func(context.Context, slowArgs) error { return nil })

	require.NoError(t, ins.Insert(ctx, db, slowArgs{}))
	require.NoError(t, ins.Insert(ctx, db, pingArgs{}))

	without := startRunner(t, db, reg)
	requireStates(t, db, "test.ping", "completed")
	require.Equal(t, []string{"available"}, jobStates(t, db, "test.slow"))

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, without.Stop(stopCtx))

	startRunner(t, db, reg, jobs.WithQueue("slow", 1))
	requireStates(t, db, "test.slow", "completed")
}

func TestInserter_DB(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	require.NoError(t, ins.Insert(context.Background(), db, pingArgs{N: 1}))
	require.Equal(t, []string{"available"}, jobStates(t, db, "test.ping"))
}

func TestInserter_UnsupportedExecutor(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ins, err := jobs.NewInserter(db)
	require.NoError(t, err)

	conn, err := db.Conn(ctx)
	require.NoError(t, err)

	defer func() { _ = conn.Close() }()

	err = ins.Insert(ctx, conn, pingArgs{N: 1})
	require.ErrorContains(t, err, "cannot insert test.ping")
	require.Empty(t, jobStates(t, db, "test.ping"))
}

type tickArgs struct{}

func (tickArgs) Kind() string { return "test.tick" }

func TestRegisterPeriodic_RunsOnStart(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	var ran atomic.Int64

	reg := newRegistry(db, fakesender.New().Delivery())
	jobs.Register(reg, func(_ context.Context, _ tickArgs) error {
		ran.Add(1)

		return nil
	})
	jobs.RegisterPeriodic(reg, time.Hour, func() river.JobArgs { return tickArgs{} })

	require.Empty(t, jobStates(t, db, "test.tick"))

	startRunner(t, db, reg)
	requireStates(t, db, "test.tick", "completed")
	require.EqualValues(t, 1, ran.Load())
}

type boomArgs struct {
	Panic bool `json:"panic"`
}

func (boomArgs) Kind() string { return "test.boom" }

func (boomArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

func TestRunner_LogsFailedAndPanickingJobs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	logger, logs := testlog.New()

	reg := newRegistry(db, fakesender.New().Delivery())
	jobs.Register(reg, func(_ context.Context, a boomArgs) error {
		if a.Panic {
			panic("kaboom")
		}

		return errors.New("failed")
	})

	insertJob(t, db, boomArgs{})
	insertJob(t, db, boomArgs{Panic: true})

	runner, err := jobs.NewRunner(db, reg,
		jobs.WithPollInterval(10*time.Millisecond), jobs.WithWorkers(2), jobs.WithRetention(time.Hour), jobs.WithLogger(logger))
	require.NoError(t, err)
	require.NoError(t, runner.Start(context.Background()))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = runner.Stop(ctx)
	})

	requireStates(t, db, "test.boom", "discarded", "discarded")
	require.Eventually(t, func() bool {
		return logs.FilterMessage("job failed").Len() == 1 && logs.FilterMessage("job panicked").Len() == 1
	}, 10*time.Second, 10*time.Millisecond)
	require.Equal(t, "test.boom", logs.FilterMessage("job failed").All()[0].ContextMap()["kind"])
}

func TestRunner_StopWithBusyJobReportsIt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	started := make(chan struct{})
	release := make(chan struct{})

	reg := newRegistry(db, fakesender.New().Delivery())
	jobs.Register(reg, func(ctx context.Context, _ pingArgs) error {
		close(started)
		<-release

		return nil
	})
	insertJob(t, db, pingArgs{})

	runner, err := jobs.NewRunner(db, reg, jobs.WithPollInterval(10*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, runner.Start(context.Background()))

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	require.ErrorContains(t, runner.Stop(ctx), "jobs: stopping with jobs still running")
	close(release)
}
