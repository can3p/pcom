package jobs

import (
	"context"
	"slices"
	"time"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/riverqueue/river"
)

// Registry collects the workers and periodic jobs a Runner runs, and what
// mail workers need: the store they record sent mail in, the delivery and
// the retry schedule.
type Registry struct {
	workers        *river.Workers
	periodic       []*river.PeriodicJob
	store          *repo.Store
	delivery       sender.Sender
	retryIntervals []time.Duration
}

// DefaultRetryIntervals is the wait before each retry of a failed mail,
// equal to the default of config.Mail.RetryIntervals. A mail fails for good
// after one attempt more than there are intervals.
var DefaultRetryIntervals = []time.Duration{10 * time.Second, 60 * time.Second, 30 * time.Minute}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithRetryIntervals sets the wait before each retry of a failed mail; a mail
// fails for good after len(intervals)+1 attempts.
func WithRetryIntervals(intervals []time.Duration) RegistryOption {
	return func(r *Registry) {
		r.retryIntervals = slices.Clone(intervals)
	}
}

// NewRegistry returns a Registry whose mail workers record sent mail in
// store and hand it to delivery. It registers the mail.rendered kind.
func NewRegistry(store *repo.Store, delivery sender.Sender, opts ...RegistryOption) *Registry {
	r := &Registry{
		workers:        river.NewWorkers(),
		store:          store,
		delivery:       delivery,
		retryIntervals: slices.Clone(DefaultRetryIntervals),
	}

	for _, o := range opts {
		o(r)
	}

	river.AddWorker(r.workers, &renderedMailWorker{deliverer: r.deliverer()})

	return r
}

// Register registers the worker of the job kind T. An error from work
// retries the job on River's default schedule; wrap it in river.JobCancel
// to fail for good.
func Register[T river.JobArgs](r *Registry, work func(ctx context.Context, args T) error) {
	river.AddWorker(r.workers, river.WorkFunc(func(ctx context.Context, job *river.Job[T]) error {
		return work(ctx, job.Args)
	}))
}

// RegisterPeriodic inserts a job built by args every interval, starting at
// the runner's start. Only the runner elected leader inserts it, so running
// several runners doesn't multiply it. Its kind must be registered too.
func RegisterPeriodic(r *Registry, every time.Duration, args func() river.JobArgs) {
	r.periodic = append(r.periodic, river.NewPeriodicJob(
		river.PeriodicInterval(every),
		func() (river.JobArgs, *river.InsertOpts) { return args(), nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	))
}
