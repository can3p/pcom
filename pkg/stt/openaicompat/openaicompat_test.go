package openaicompat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/stt"
	"github.com/can3p/pcom/pkg/stt/openaicompat"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/stretchr/testify/require"
	wm "github.com/wiremock/go-wiremock"
)

const (
	apiKey = "test-key"
	model  = "whisper-test"
)

func TestMain(m *testing.M) {
	wiremock.Register("testdata/wiremock")

	code := m.Run()
	_ = wiremock.Cleanup()

	os.Exit(code)
}

// backend talks to the shared WireMock, wrapped in the shared retries that
// record their waits instead of waiting; the test fails on a request no stub matches.
func backend(t *testing.T, key string) (stt.Backend, *wiremock.WireMock, *[]time.Duration) {
	t.Helper()

	if testing.Short() {
		t.Skip("starts a container")
	}

	w := wiremock.Shared(t)
	b := openaicompat.New(openaicompat.Options{Endpoint: w.Watch(t, "stt"), Model: model, Key: key})
	waits := new([]time.Duration)
	record := func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	}

	return stt.Retry(b, stt.RetryOptions{Sleep: record}), w, waits
}

// sent counts the requests whose multipart body contains every text.
func sent(texts ...string) *wm.Request {
	r := wm.NewRequest("POST", wm.URLPathEqualTo("/stt/audio/transcriptions"))
	for _, s := range texts {
		r = r.WithBodyPattern(wm.Contains(s))
	}

	return r
}

func TestTranscribe_SuccessSendsFileModelAndFormatOnly(t *testing.T) {
	b, w, _ := backend(t, apiKey)

	res, err := b.Transcribe(t.Context(), stt.Audio{Bytes: []byte("wm-ok-plain"), ContentType: "audio/webm;codecs=opus"})
	require.NoError(t, err)
	require.Equal(t, stt.Result{
		Text: "This is a local transcript of 12 seconds of audio. #local", Language: "english", Seconds: 12,
	}, res)
	require.Equal(t, "openai-compat", b.Name())
	require.Equal(t, model, b.Model())

	w.Verify(t, 1, sent("wm-ok-plain", `name="model"`, model, `name="response_format"`, "verbose_json", `name="file"; filename="audio.webm"`))
	w.Verify(t, 0, sent("wm-ok-plain", `name="language"`))
}

func TestTranscribe_LanguageIsSentOnlyWhenForced(t *testing.T) {
	b, w, _ := backend(t, apiKey)

	res, err := b.Transcribe(t.Context(), stt.Audio{Bytes: []byte("wm-forced"), ContentType: "audio/mp4", Language: "ru"})
	require.NoError(t, err)
	require.Equal(t, "russian", res.Language)
	w.Verify(t, 1, sent("wm-forced", "name=\"language\"\r\n\r\nru\r\n", `filename="audio.m4a"`))
}

func TestTranscribe_RateLimitThenSuccess(t *testing.T) {
	b, w, waits := backend(t, apiKey)

	res, err := b.Transcribe(t.Context(), stt.Audio{Bytes: []byte("wm-retry"), ContentType: "audio/webm"})
	require.NoError(t, err)
	require.Equal(t, "After the retry.", res.Text)
	require.InDelta(t, 3.5, res.Seconds, 0)
	require.Equal(t, []time.Duration{7 * time.Second}, *waits, "the Retry-After header is what the retry waits")
	w.Verify(t, 2, sent("wm-retry"))
}

func TestTranscribe_ServerErrorExhaustsAttempts(t *testing.T) {
	b, w, _ := backend(t, apiKey)

	_, err := b.Transcribe(t.Context(), stt.Audio{Bytes: []byte("wm-5xx"), ContentType: "audio/webm"})

	var se *stt.StatusError

	require.ErrorAs(t, err, &se)
	require.Equal(t, 503, se.Status)
	w.Verify(t, stt.DefaultAttempts, sent("wm-5xx"))
}

func TestTranscribe_BadKeyIsNotRetried(t *testing.T) {
	b, w, _ := backend(t, "wrong-key")

	_, err := b.Transcribe(t.Context(), stt.Audio{Bytes: []byte("wm-bad-key"), ContentType: "audio/webm"})

	var se *stt.StatusError

	require.ErrorAs(t, err, &se)
	require.Equal(t, 401, se.Status)
	require.False(t, se.Retryable())
	w.Verify(t, 1, sent("wm-bad-key"))
}
