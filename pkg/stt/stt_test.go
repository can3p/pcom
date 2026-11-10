package stt_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/stt"
	"github.com/stretchr/testify/require"
)

// script is a backend answering with the next error of its list, then ok.
type script struct {
	errs  []error
	calls int
}

func (s *script) Name() string  { return "script" }
func (s *script) Model() string { return "m" }

func (s *script) Transcribe(context.Context, stt.Audio) (stt.Result, error) {
	defer func() { s.calls++ }()

	if s.calls < len(s.errs) {
		return stt.Result{}, s.errs[s.calls]
	}

	return stt.Result{Text: "ok"}, nil
}

func TestRetry(t *testing.T) {
	t.Parallel()

	limited := &stt.StatusError{Status: 429, RetryAfter: 7 * time.Second}
	down := &stt.StatusError{Status: 502}
	denied := &stt.StatusError{Status: 401}
	boom := errors.New("boom")

	cases := []struct {
		name      string
		errs      []error
		wantErr   error
		wantCalls int
		wantWaits []time.Duration
	}{
		{"success needs no retry", nil, nil, 1, nil},
		{"Retry-After is the wait", []error{limited}, nil, 2, []time.Duration{7 * time.Second}},
		{"server errors back off exponentially", []error{down, down}, nil, 3, []time.Duration{time.Second, 2 * time.Second}},
		{"the last error is returned", []error{down, down, down}, down, 3, []time.Duration{time.Second, 2 * time.Second}},
		{"a bad key is not retried", []error{denied}, denied, 1, nil},
		{"another error is not retried", []error{boom}, boom, 1, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var waits []time.Duration

			b := &script{errs: tc.errs}
			r := stt.Retry(b, stt.RetryOptions{Sleep: func(_ context.Context, d time.Duration) error {
				waits = append(waits, d)
				return nil
			}})

			_, err := r.Transcribe(t.Context(), stt.Audio{})
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}

			require.Equal(t, tc.wantCalls, b.calls)
			require.Equal(t, tc.wantWaits, waits)
		})
	}
}

func TestRetry_WaitIsCapped(t *testing.T) {
	t.Parallel()

	var waits []time.Duration

	limited := &stt.StatusError{Status: 429, RetryAfter: time.Hour}
	r := stt.Retry(&script{errs: []error{limited}}, stt.RetryOptions{Sleep: func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}})

	_, err := r.Transcribe(t.Context(), stt.Audio{})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{stt.DefaultMaxDelay}, waits)
}

func TestRetry_CancelledContextStopsTheWait(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	b := &script{errs: []error{&stt.StatusError{Status: 429}}}

	_, err := stt.Retry(b, stt.RetryOptions{}).Transcribe(ctx, stt.Audio{})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, b.calls)
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		header string
		want   time.Duration
	}{
		{"7", 7 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"Sat, 07 Nov 2026 12:00:30 GMT", 30 * time.Second},
		{"Sat, 07 Nov 2026 11:00:00 GMT", 0},
		{"soon", 0},
		{"", 0},
	}

	for _, tc := range cases {
		require.Equal(t, tc.want, stt.ParseRetryAfter(tc.header, now), tc.header)
	}
}

func TestRegistry(t *testing.T) {
	t.Parallel()

	reg := stt.Registry{"script": func() (stt.Backend, error) { return &script{}, nil }}

	b, err := reg.Backend("")
	require.NoError(t, err)
	require.Nil(t, b, "no provider turns transcription off")

	b, err = reg.Backend("script")
	require.NoError(t, err)
	require.Equal(t, "script", b.Name())

	_, err = reg.Backend("whisper.cpp")
	require.ErrorContains(t, err, "unknown provider")
}
