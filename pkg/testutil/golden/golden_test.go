package golden_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/stretchr/testify/require"
)

func TestAssert_WritesWithUpdateGolden(t *testing.T) {
	// Not t.Parallel(): shares UPDATE_GOLDEN and a testdata file with the
	// other tests in this file.
	const name = "selftest-write"
	path := filepath.Join("testdata", name+".golden")
	t.Cleanup(func() { _ = os.Remove(path) })

	t.Setenv("UPDATE_GOLDEN", "1")
	golden.Assert(t, name, []byte("hello golden"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello golden", string(got))
}

func TestAssert_MatchesAndMismatches(t *testing.T) {
	const name = "selftest-compare"
	path := filepath.Join("testdata", name+".golden")
	t.Cleanup(func() { _ = os.Remove(path) })

	t.Setenv("UPDATE_GOLDEN", "1")
	golden.Assert(t, name, []byte("expected content"))

	t.Setenv("UPDATE_GOLDEN", "")
	golden.Assert(t, name, []byte("expected content")) // matches: must not fail

	ft := &fakeTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		golden.Assert(ft, name, []byte("different content"))
	}()
	<-done

	require.True(t, ft.failed, "Assert should fail the test on a mismatch")
}

func TestAssert_FailsOnMissingFile(t *testing.T) {
	t.Setenv("UPDATE_GOLDEN", "")

	ft := &fakeTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		golden.Assert(ft, "selftest-does-not-exist", []byte("anything"))
	}()
	<-done

	require.True(t, ft.failed, "Assert should fail the test when the golden file is missing")
}

// fakeTB records a Fatalf call instead of failing the real test, so a test
// can assert that Assert's failure path runs without failing itself.
type fakeTB struct {
	failed bool
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	runtime.Goexit()
}
