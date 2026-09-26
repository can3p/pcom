package testutil_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/can3p/pcom/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func TestMust_PassesThroughOnSuccess(t *testing.T) {
	t.Parallel()

	got := testutil.Must(t, 42, nil)
	require.Equal(t, 42, got)
}

func TestMust_FailsTestOnError(t *testing.T) {
	t.Parallel()

	ft := &fakeTB{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		testutil.Must(ft, 0, errors.New("boom"))
		ft.reachedAfter = true
	}()
	<-done

	require.True(t, ft.failed, "Must should have called Fatalf")
	require.False(t, ft.reachedAfter, "Fatalf should stop the goroutine, like testing.T's does")
}

// fakeTB records a Fatalf call instead of failing the real test, so a test
// can assert that Must's failure path runs without failing itself.
type fakeTB struct {
	failed       bool
	reachedAfter bool
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	runtime.Goexit()
}
