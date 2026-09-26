// Package factory builds valid rows for pcom's tables so that tests (and
// cmd/seed) don't have to know the schema. Every builder inserts through the
// ORM and returns the row it created; defaults satisfy every constraint and
// are unique across a run, so a test only overrides what it cares about.
//
// Tests reach the ORM only through this package (see docs/testing.md): a
// test body should not call core.Posts(...) or similar directly.
//
// This package must not import "testing": cmd/seed links it into a normal
// binary, not a test binary.
package factory

import (
	"sync/atomic"

	"github.com/google/uuid"
)

// counter hands out small increasing integers so that every made-up value
// (an email, a username, a URL, ...) is unique across a whole test run
// without a caller having to think about it.
var counter int64

func next() int64 {
	return atomic.AddInt64(&counter, 1)
}

// newID mints a time-ordered UUID for a new row's primary key, the same way
// pcom's own write paths do (see e.g. userops.CreateConnection).
func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	return id.String(), nil
}
