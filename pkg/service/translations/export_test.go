package translations

import "context"

// RunJobs sends the due jobs once, as one tick of Run.
func (s *Service) RunJobs(ctx context.Context) error { return s.runJobs(ctx) }
