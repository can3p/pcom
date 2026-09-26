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

// Must returns v when err is nil. Otherwise it fails the test immediately,
// the way t.Fatalf would. It is for one-line fixture setup:
//
//	body := testutil.Must(t, os.ReadFile(path))
func Must[T any](t TB, v T, err error) T {
	t.Helper()

	if err != nil {
		t.Fatalf("testutil.Must: unexpected error: %v", err)
	}

	return v
}
