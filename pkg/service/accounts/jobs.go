package accounts

import (
	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/repo"
)

// WithJobs sets the queue the service enqueues its background jobs on.
func WithJobs(q repo.JobQueue) Option {
	return func(s *Service) {
		s.jobs = q
	}
}

// RegisterJobs registers the workers of the service's job kinds on r.
func (s *Service) RegisterJobs(r *jobs.Registry) {}
