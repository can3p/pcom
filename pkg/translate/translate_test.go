package translate_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/stretchr/testify/require"
)

// fake is an in-memory backend: it upper-cases text outside tags, records
// every call, and answers with the queued errors first.
type fake struct {
	limits translate.Limits
	mu     sync.Mutex
	calls  [][]string
	errs   []error
	mangle func([]string) []string
}

func (*fake) Name() string               { return "fake" }
func (*fake) DisplayName() string        { return "Fake Translator" }
func (f *fake) Limits() translate.Limits { return f.limits }

func (f *fake) Translate(_ context.Context, req translate.Request) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, req.Segments)

	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]

		return nil, err
	}

	out := make([]string, len(req.Segments))
	for i, s := range req.Segments {
		out[i] = "EN:" + s
	}

	if f.mangle != nil {
		out = f.mangle(out)
	}

	return out, nil
}

// noSleep records the waits instead of sleeping.
func noSleep(waits *[]time.Duration) translate.Option {
	return translate.WithSleep(func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	})
}

func TestChunk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		segs   []string
		limits translate.Limits
		want   [][]string
		err    error
	}{
		{"no limits", []string{"a", "b", "c"}, translate.Limits{}, [][]string{{"a", "b", "c"}}, nil},
		{"segment limit", []string{"a", "b", "c"}, translate.Limits{Segments: 2}, [][]string{{"a", "b"}, {"c"}}, nil},
		{"exactly at the char limit", []string{"ab", "cd", "e"}, translate.Limits{Chars: 4}, [][]string{{"ab", "cd"}, {"e"}}, nil},
		{"chars counted in runes", []string{"жж", "ёё"}, translate.Limits{Chars: 4}, [][]string{{"жж", "ёё"}}, nil},
		{"segment limit cuts first", []string{"a", "b", "c"}, translate.Limits{Segments: 2, Chars: 4}, [][]string{{"a", "b"}, {"c"}}, nil},
		{"char limit cuts first", []string{"aaa", "bb", "c"}, translate.Limits{Segments: 3, Chars: 4}, [][]string{{"aaa"}, {"bb", "c"}}, nil},
		{"segment over the char limit", []string{"a", "toolong"}, translate.Limits{Chars: 4}, nil, translate.ErrTooLong},
		{"empty", nil, translate.Limits{Chars: 4}, nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := translate.Chunk(tc.segs, tc.limits)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTranslator_ChunksAndJoinsInOrder(t *testing.T) {
	t.Parallel()

	b := &fake{limits: translate.Limits{Segments: 2, Chars: 6}}
	tr := translate.NewTranslator(b)

	got, err := tr.Translate(context.Background(), translate.Request{From: "ru", To: "en", Segments: []string{"aaa", "bbb", "c", "d", "e"}})
	require.NoError(t, err)
	require.Equal(t, []string{"EN:aaa", "EN:bbb", "EN:c", "EN:d", "EN:e"}, got)
	require.Equal(t, [][]string{{"aaa", "bbb"}, {"c", "d"}, {"e"}}, b.calls)
	require.Equal(t, "fake", tr.Name())
	require.Equal(t, "Fake Translator", tr.DisplayName())
}

func TestTranslator_Retries(t *testing.T) {
	t.Parallel()

	limited := &translate.StatusError{Status: http.StatusTooManyRequests, RetryAfter: 7 * time.Second}
	unavailable := &translate.StatusError{Status: http.StatusServiceUnavailable}
	unauthorized := &translate.StatusError{Status: http.StatusUnauthorized, Code: "401000"}
	errNetwork := errors.New("dial tcp: connection refused")

	cases := []struct {
		name  string
		errs  []error
		calls int
		waits []time.Duration
		err   error
	}{
		{"429 honours Retry-After", []error{limited}, 2, []time.Duration{7 * time.Second}, nil},
		{"5xx backs off exponentially", []error{unavailable, unavailable}, 3, []time.Duration{time.Second, 2 * time.Second}, nil},
		{"Retry-After is capped", []error{&translate.StatusError{Status: 429, RetryAfter: time.Hour}}, 2, []time.Duration{time.Minute}, nil},
		{"gives up after the last attempt", []error{unavailable, unavailable, unavailable, unavailable}, 4, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, unavailable},
		{"401 is not retried", []error{unauthorized}, 1, nil, unauthorized},
		{"a network error is not retried", []error{errNetwork}, 1, nil, errNetwork},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var waits []time.Duration

			b := &fake{errs: tc.errs}
			got, err := translate.NewTranslator(b, noSleep(&waits)).Translate(context.Background(), translate.Request{To: "en", Segments: []string{"x"}})

			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"EN:x"}, got)
			}

			require.Len(t, b.calls, tc.calls)
			require.Equal(t, tc.waits, waits)
		})
	}
}

func TestTranslator_CancelledWhileWaiting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	b := &fake{errs: []error{&translate.StatusError{Status: 429}}}
	_, err := translate.NewTranslator(b, translate.WithBackoff(time.Hour, time.Hour)).Translate(ctx, translate.Request{Segments: []string{"x"}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestTranslator_RejectsWrongShape(t *testing.T) {
	t.Parallel()

	src := []string{
		`Hello <span class="notranslate" data-i="0">@alice</span> and <em>you</em>`,
		`see <span class="notranslate" data-i="1"><code>x &lt; y</code></span>`,
	}

	cases := []struct {
		name   string
		mangle func([]string) []string
		err    string
	}{
		{"unchanged shape passes", nil, ""},
		{"spans may move and be requoted", func(s []string) []string {
			s[0] = `<span data-i='0' class="notranslate">@alice</span> hi and <em>you</em>`
			return s
		}, ""},
		{"a segment fewer", func(s []string) []string { return s[:1] }, "1 segments, want 2"},
		{"a segment more", func(s []string) []string { return append(s, "extra") }, "3 segments, want 2"},
		{"lost span", func(s []string) []string {
			s[0] = "Hello @alice"
			return s
		}, `segment 0 lost protected span data-i="0"`},
		{"changed span", func(s []string) []string {
			s[1] = strings.Replace(s[1], "x &lt; y", "x &lt; z", 1)
			return s
		}, `segment 1 changed protected span data-i="1"`},
		{"extra span", func(s []string) []string {
			s[0] += `<span class="notranslate" data-i="9">new</span>`
			return s
		}, "segment 0 has 2 protected spans, want 1"},
		{"unclosed span", func(s []string) []string {
			s[0] = `Hello <span class="notranslate" data-i="0">@alice`
			return s
		}, "is not closed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := &fake{mangle: tc.mangle}
			got, err := translate.NewTranslator(b).Translate(context.Background(), translate.Request{To: "en", Segments: src})

			if tc.err == "" {
				require.NoError(t, err)
				require.Len(t, got, len(src))

				return
			}

			require.ErrorIs(t, err, translate.ErrShape)
			require.ErrorContains(t, err, tc.err)
			require.Nil(t, got, "never a partial translation")
		})
	}
}

func TestTranslator_SegmentTooLong(t *testing.T) {
	t.Parallel()

	b := &fake{limits: translate.Limits{Chars: 3}}
	_, err := translate.NewTranslator(b).Translate(context.Background(), translate.Request{Segments: []string{"ok", "toolong"}})
	require.ErrorIs(t, err, translate.ErrTooLong)
	require.Empty(t, b.calls, "nothing is sent")
}

func TestTranslator_NoPartialResultAcrossChunks(t *testing.T) {
	t.Parallel()

	calls := 0
	b := &fake{limits: translate.Limits{Segments: 1}, mangle: func(s []string) []string {
		calls++
		if calls == 2 {
			return nil
		}

		return s
	}}

	got, err := translate.NewTranslator(b).Translate(context.Background(), translate.Request{Segments: []string{"a", "b"}})
	require.ErrorIs(t, err, translate.ErrShape)
	require.Nil(t, got)
}

var registrations atomic.Int32

func TestNew(t *testing.T) {
	t.Parallel()

	tr, err := translate.New(config.Translation{})
	require.NoError(t, err)
	require.Nil(t, tr, "an empty provider turns translation off")

	_, err = translate.New(config.Translation{Provider: "nonesuch"})
	require.ErrorContains(t, err, `no backend "nonesuch"`)

	name := fmt.Sprintf("fake-%d", registrations.Add(1)) // the registry is global: unique per run for -count
	translate.Register(name, func(config.Translation) (translate.Backend, error) { return &fake{}, nil })
	require.Contains(t, translate.Registered(), name)
	require.Panics(t, func() { translate.Register(name, nil) })

	tr, err = translate.New(config.Translation{Provider: name})
	require.NoError(t, err)
	require.Equal(t, "Fake Translator", tr.DisplayName())
}

func TestChars(t *testing.T) {
	t.Parallel()

	require.Equal(t, 7, translate.Chars("abc", "жжж", "<"))
}

func TestDetectLanguage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text string
		want string
		ok   bool
	}{
		{"Сегодня мы ходили в лес за грибами и нашли очень много белых грибов.", "ru", true},
		{"Today we went to the forest to look for mushrooms and found a great many of them.", "en", true},
		{"Heute sind wir in den Wald gegangen, um Pilze zu suchen, und haben sehr viele gefunden.", "de", true},
		{"ok", "", false},
	}

	for _, tc := range cases {
		got, ok := translate.DetectLanguage(tc.text)
		require.Equal(t, tc.ok, ok, tc.text)
		require.Equal(t, tc.want, got, tc.text)
	}
}
