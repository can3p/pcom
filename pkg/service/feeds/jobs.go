package feeds

import (
	"time"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// DefaultPollInterval is how often due feeds are looked for, equal to the
// default of config.Feeds.PollInterval.
const DefaultPollInterval = 10 * time.Second

// WithJobs sets the queue the service enqueues its background jobs on.
func WithJobs(q repo.JobQueue) Option {
	return func(s *Service) {
		s.jobs = q
	}
}

// WithPollInterval sets how often the feeds.schedule job looks for due
// feeds; zero keeps DefaultPollInterval.
func WithPollInterval(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.pollInterval = d
		}
	}
}

// ScheduleArgs is the periodic job that queues a refresh of every due feed.
type ScheduleArgs struct{}

func (ScheduleArgs) Kind() string { return "feeds.schedule" }

// InsertOpts gives the schedule a single attempt: the next one is at most a
// poll interval away.
func (ScheduleArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

// RefreshArgs is the job that fetches one feed and stores its new items.
type RefreshArgs struct {
	FeedID string `json:"feed_id"`
}

func (RefreshArgs) Kind() string { return "feeds.refresh" }

// InsertOpts makes a refresh unique by feed among unfinished jobs, so a feed
// slower to refresh than the poll interval isn't queued twice.
func (RefreshArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{
		ByArgs: true,
		ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRetryable,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
		},
	}}
}

// RegisterJobs registers the workers of the service's job kinds on r, and
// the schedule that inserts feeds.schedule every poll interval.
func (s *Service) RegisterJobs(r *jobs.Registry) {
	jobs.Register(r, s.ScheduleRefreshes)
	jobs.Register(r, s.Refresh)
	jobs.RegisterPeriodic(r, s.pollInterval, func() river.JobArgs { return ScheduleArgs{} })
}
