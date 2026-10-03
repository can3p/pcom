package wiremock_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/stretchr/testify/require"
	wm "github.com/wiremock/go-wiremock"
)

func TestMain(m *testing.M) {
	wiremock.Register("testdata")

	code := m.Run()
	_ = wiremock.Cleanup()

	os.Exit(code)
}

// fakeT records what a test would report; Cleanup functions run on demand.
type fakeT struct {
	testing.TB
	errors   []string
	cleanups []func()
}

func (f *fakeT) Helper()                   {}
func (f *fakeT) Cleanup(fn func())         { f.cleanups = append(f.cleanups, fn) }
func (f *fakeT) Errorf(s string, a ...any) { f.errors = append(f.errors, fmt.Sprintf(s, a...)) }

func post(t testing.TB, url, body string) (int, string) {
	t.Helper()

	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(data)
}

func TestToyMappingIsServedAndVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	w := wiremock.Shared(t)
	url := w.Watch(t, "toy") + "/translate"

	code, body := post(t, url, `{"text": "hello toy"}`)
	require.Equal(t, 200, code)
	require.JSONEq(t, `{"text": "bonjour toy"}`, body)

	pattern := wm.NewRequest("POST", wm.URLPathEqualTo("/toy/translate")).WithBodyPattern(wm.EqualToJson(`{"text": "hello toy"}`))
	w.Verify(t, 1, pattern)

	other := wm.NewRequest("POST", wm.URLPathEqualTo("/toy/translate")).WithBodyPattern(wm.EqualToJson(`{"text": "never sent"}`))
	w.Verify(t, 0, other)
}

func TestUnmatchedCallFailsTheTest(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	w := wiremock.Shared(t)
	f := &fakeT{TB: t}
	url := w.Watch(f, "toy-unmatched") + "/translate"

	code, _ := post(t, url, `{"text": "no stub for this"}`)
	require.Equal(t, 404, code)
	require.Empty(t, f.errors)

	for _, fn := range f.cleanups {
		fn()
	}

	require.Len(t, f.errors, 1)
	require.Contains(t, f.errors[0], "no stub for this")

	// a test watching another prefix is not failed by it
	g := &fakeT{TB: t}
	w.Watch(g, "toy")

	for _, fn := range g.cleanups {
		fn()
	}

	require.Empty(t, g.errors)

	// with markers, only the unmatched requests carrying one count
	h := &fakeT{TB: t}
	w.Watch(h, "toy-unmatched", "some other test's text")

	for _, fn := range h.cleanups {
		fn()
	}

	require.Empty(t, h.errors)

	k := &fakeT{TB: t}
	w.Watch(k, "toy-unmatched", "no stub for")

	for _, fn := range k.cleanups {
		fn()
	}

	require.NotEmpty(t, k.errors)
}
