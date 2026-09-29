// Package testutil holds the smallest, most widely used test helper:
// Must. Everything else (fakes, gin contexts, golden files, a test
// database) lives in its own subpackage so a test imports only what it
// needs.
package testutil

// TB is the subset of testing.TB that Must needs. *testing.T and *testing.B
// satisfy it. A test that wants to assert on Must's own failure path (as
// opposed to using Must to set up a fixture) can pass a fake that records
// the call instead of stopping the real test.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Must checks the (value, error) pair a fixture call returns and gives back
// a function that yields the value, or fails the test the way t.Fatalf would.
// The pair must be Must's only arguments, because Go spreads a multi-value
// call into another call only when it is the sole argument, so t comes after:
//
//	user := testutil.Must(factory.User(ctx, db))(t)
func Must[T any](v T, err error) func(t TB) T {
	return func(t TB) T {
		t.Helper()

		if err != nil {
			t.Fatalf("testutil.Must: unexpected error: %v", err)
		}

		return v
	}
}
