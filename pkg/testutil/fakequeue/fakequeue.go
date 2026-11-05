// Package fakequeue is a fake of github.com/can3p/pcom/pkg/repo.JobQueue for
// unit and package tests: it records the args of every job instead of
// inserting it. A service test asserts on the args a change enqueued; what a
// mail job then says is tested on the service's render method.
package fakequeue

import (
	"context"
	"sync"

	"github.com/can3p/pcom/pkg/repo"
)

// compile-time check that Queue implements repo.JobQueue.
var _ repo.JobQueue = (*Queue)(nil)

// Queue records every job inserted. The zero value is ready to use; it is
// safe for concurrent use.
type Queue struct {
	mu   sync.Mutex
	jobs []repo.JobArgs
	err  error
}

// New returns a Queue that records jobs and never fails.
func New() *Queue {
	return &Queue{}
}

// FailWith makes every subsequent Insert return err instead of recording the
// job. Pass nil to go back to recording.
func (q *Queue) FailWith(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.err = err
}

// Insert implements repo.JobQueue. exec is not used: the fake records the
// job whether or not the caller's transaction commits.
func (q *Queue) Insert(_ context.Context, _ repo.Executor, args repo.JobArgs) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.err != nil {
		return q.err
	}

	q.jobs = append(q.jobs, args)

	return nil
}

// Inserted returns the args of every job inserted so far, in order.
func (q *Queue) Inserted() []repo.JobArgs {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]repo.JobArgs, len(q.jobs))
	copy(out, q.jobs)

	return out
}

// Of returns the args of the jobs of type T inserted so far, in order.
func Of[T repo.JobArgs](q *Queue) []T {
	var out []T

	for _, a := range q.Inserted() {
		if t, ok := a.(T); ok {
			out = append(out, t)
		}
	}

	return out
}
