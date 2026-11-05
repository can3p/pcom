// Package jobs runs pcom's background work on River
// (github.com/riverqueue/river), over the same *sql.DB the services use.
//
// A service enqueues a job with tx.Enqueue(ctx, queue, args) in the
// transaction of the change that causes it, so the job exists only if that
// change commits; the queue is an Inserter, which inserts on the
// transaction. The work itself runs in a Runner, which `web serve` starts:
// every service that has jobs registers its workers on a Registry with a
// RegisterJobs method.
//
// Adding a job kind: declare its args struct in the service that owns the
// work, with a Kind method returning "<area>.<name>" (mail kinds are
// "mail.<email_type>") and, when it needs any, an InsertOpts method for
// uniqueness or attempts; register its worker in the service's RegisterJobs
// with Register, RegisterMail or RegisterPeriodic. The args are stored as
// JSON, so they hold ids and the few values the database doesn't keep, and
// the worker loads the rest when it runs.
//
// River's tables come from migrations/*-river-NNN.sql, one file per River
// migration version, exported from River's own migrations with
// rivermigrate.AllVersions; a River upgrade that adds a migration exports the
// new version the same way, into a file of its own.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Defaults of the runner's settings, equal to the defaults of config.Jobs.
const (
	// DefaultPollInterval is how often the runner looks for jobs to work.
	DefaultPollInterval = time.Second
	// DefaultWorkers is how many jobs the runner works at once.
	DefaultWorkers = 10
	// DefaultRetention is how long finished jobs are kept.
	DefaultRetention = 24 * time.Hour
)

// riverLogLevel is the least level of River's own log records that reach
// the logger: its info records narrate every start, stop and election.
const riverLogLevel = zapcore.WarnLevel

// riverFetchCooldown is River's default FetchCooldown, which may not exceed
// the poll interval: a shorter poll interval (tests) shortens it too.
const riverFetchCooldown = 100 * time.Millisecond

// Inserter inserts jobs; it works none. It is the repo.JobQueue services
// get.
type Inserter struct {
	client *river.Client[*sql.Tx]
}

// compile-time check that Inserter is a job queue.
var _ repo.JobQueue = (*Inserter)(nil)

// NewInserter returns an Inserter over db.
func NewInserter(db *sql.DB) (*Inserter, error) {
	client, err := river.NewClient(riverdatabasesql.New(db), &river.Config{
		Logger: logging.Slog(zap.NewNop(), riverLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: building the inserter: %w", err)
	}

	return &Inserter{client: client}, nil
}

// Insert inserts a job on exec: inside the transaction when exec is one,
// on its own otherwise.
func (i *Inserter) Insert(ctx context.Context, exec repo.Executor, args repo.JobArgs) error {
	var err error

	switch e := exec.(type) {
	case *sql.Tx:
		_, err = i.client.InsertTx(ctx, e, args, nil)
	case *sql.DB:
		_, err = i.client.Insert(ctx, args, nil)
	default:
		return fmt.Errorf("jobs: cannot insert %s on a %T", args.Kind(), exec)
	}

	if err != nil {
		return fmt.Errorf("jobs: inserting %s: %w", args.Kind(), err)
	}

	return nil
}

// Runner works the jobs of every kind registered on its Registry.
type Runner struct {
	client *river.Client[*sql.Tx]
}

type runnerConfig struct {
	pollInterval time.Duration
	workers      int
	retention    time.Duration
	logger       *zap.Logger
}

// Option configures a Runner.
type Option func(*runnerConfig)

// WithPollInterval sets how often the runner looks for jobs to work.
func WithPollInterval(d time.Duration) Option {
	return func(c *runnerConfig) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithWorkers sets how many jobs the runner works at once.
func WithWorkers(n int) Option {
	return func(c *runnerConfig) {
		if n > 0 {
			c.workers = n
		}
	}
}

// WithRetention sets how long finished jobs (completed, cancelled or
// discarded) are kept.
func WithRetention(d time.Duration) Option {
	return func(c *runnerConfig) {
		if d > 0 {
			c.retention = d
		}
	}
}

// WithLogger sets the logger failed and panicking jobs are logged to; the
// default logs nothing.
func WithLogger(l *zap.Logger) Option {
	return func(c *runnerConfig) {
		c.logger = l
	}
}

// NewRunner returns a Runner over db for the kinds registered on reg. It
// polls: River's database/sql driver has no LISTEN.
func NewRunner(db *sql.DB, reg *Registry, opts ...Option) (*Runner, error) {
	cfg := runnerConfig{
		pollInterval: DefaultPollInterval,
		workers:      DefaultWorkers,
		retention:    DefaultRetention,
		logger:       zap.NewNop(),
	}

	for _, o := range opts {
		o(&cfg)
	}

	client, err := river.NewClient(riverdatabasesql.New(db), &river.Config{
		CancelledJobRetentionPeriod: cfg.retention,
		CompletedJobRetentionPeriod: cfg.retention,
		DiscardedJobRetentionPeriod: cfg.retention,
		ErrorHandler:                errorLogger{logger: cfg.logger},
		FetchPollInterval:           cfg.pollInterval,
		FetchCooldown:               min(cfg.pollInterval, riverFetchCooldown),
		Logger:                      logging.Slog(cfg.logger, riverLogLevel),
		PeriodicJobs:                reg.periodic,
		PollOnly:                    true,
		Queues:                      map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.workers}},
		Workers:                     reg.workers,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: building the runner: %w", err)
	}

	return &Runner{client: client}, nil
}

// Start starts working jobs in the background; Stop ends it.
func (r *Runner) Start(ctx context.Context) error {
	return r.client.Start(ctx)
}

// Stop stops fetching jobs and waits, until ctx is done, for the jobs being
// worked to finish.
func (r *Runner) Stop(ctx context.Context) error {
	err := r.client.Stop(ctx)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("jobs: stopping with jobs still running: %w", err)
	}

	return err
}

// errorLogger logs every failed or panicking job and leaves River's retry
// decision alone.
type errorLogger struct {
	logger *zap.Logger
}

func (e errorLogger) HandleError(_ context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	e.logger.Warn("job failed", zap.String("kind", job.Kind), zap.Int64("job_id", job.ID), zap.Int("attempt", job.Attempt), zap.Error(err))

	return nil
}

func (e errorLogger) HandlePanic(_ context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	e.logger.Error("job panicked", zap.String("kind", job.Kind), zap.Int64("job_id", job.ID), zap.Int("attempt", job.Attempt), zap.Any("panic", panicVal), zap.String("trace", trace))

	return nil
}
