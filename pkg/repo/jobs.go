package repo

import "context"

// JobArgs is a background job's arguments: a struct River stores as JSON,
// named by its Kind. It is river.JobArgs without the import; a job's insert
// options (max attempts, uniqueness) come from its InsertOpts method, as
// River reads them.
type JobArgs interface {
	Kind() string
}

// JobQueue inserts a job on the caller's executor, so the job exists only if
// the transaction commits. pkg/jobs implements it over River; fakequeue
// records the args for tests.
type JobQueue interface {
	Insert(ctx context.Context, exec Executor, args JobArgs) error
}

// Enqueue inserts a job on the store's executor: a service that enqueues
// inside Tx enqueues only if the transaction commits.
func (s *Store) Enqueue(ctx context.Context, q JobQueue, args JobArgs) error {
	return q.Insert(ctx, s.exec, args)
}
