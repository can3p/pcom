package azure_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/can3p/pcom/pkg/translate/azure"
	"github.com/stretchr/testify/require"
)

const prefix = "azure-translator"

func TestMain(m *testing.M) {
	wiremock.Register("testdata/wiremock")

	code := m.Run()
	_ = wiremock.Cleanup()

	os.Exit(code)
}

// backend points at the stubs; it fails the test for a call no stub matched.
func backend(t *testing.T, key, region string) *azure.Backend {
	t.Helper()

	if testing.Short() {
		t.Skip("needs the WireMock container")
	}

	endpoint := wiremock.Shared(t).Watch(t, prefix)

	return azure.New(config.AzureTranslator{Key: settings.Secret(key), Region: region, Endpoint: endpoint + "/"}, nil)
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		region string
		req    translate.Request
		want   []string
	}{
		{"global resource, HTML with a protected span", "", translate.Request{From: "ru", To: "en", Segments: []string{
			`Привет, <span class="notranslate" data-i="0">@azure-stub-alice</span>!`,
			"Мы <em>ходили</em> в лес.",
		}}, []string{
			`Hello, <span class="notranslate" data-i="0">@azure-stub-alice</span>!`,
			"We <em>went</em> to the forest.",
		}},
		{"regional resource sends its region", "westeurope", translate.Request{From: "de", To: "en", Segments: []string{"Guten Tag, azure-stub-region"}},
			[]string{"Good day, azure-stub-region"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := translate.NewTranslator(backend(t, "azure-test-key", tc.region)).Translate(context.Background(), tc.req)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTranslate_Unauthorized(t *testing.T) {
	t.Parallel()

	_, err := backend(t, "", "").Translate(context.Background(), translate.Request{From: "ru", To: "en", Segments: []string{"azure-stub-nokey"}})

	var se *translate.StatusError
	require.ErrorAs(t, err, &se)
	require.Equal(t, http.StatusUnauthorized, se.Status)
	require.Equal(t, "401000", se.Code)
	require.False(t, se.Retryable())
}

func TestTranslate_Retries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
		wait time.Duration
	}{
		{"429 then 200", "azure-stub-retry: Привет", "azure-stub-retry: Hello", 3 * time.Second},
		{"503 then 200", "azure-stub-retry-503: Привет", "azure-stub-retry-503: Hello", 2 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var waits []time.Duration

			tr := translate.NewTranslator(backend(t, "azure-test-key", ""), translate.WithSleep(func(_ context.Context, d time.Duration) error {
				waits = append(waits, d)
				return nil
			}))

			got, err := tr.Translate(context.Background(), translate.Request{From: "ru", To: "en", Segments: []string{tc.text}})
			require.NoError(t, err)
			require.Equal(t, []string{tc.want}, got)
			require.Equal(t, []time.Duration{tc.wait}, waits, "one retry, after Retry-After")
		})
	}
}

func TestTranslate_WrongElementCount(t *testing.T) {
	t.Parallel()

	_, err := backend(t, "azure-test-key", "").Translate(context.Background(), translate.Request{From: "ru", To: "en",
		Segments: []string{"azure-stub-short: раз", "azure-stub-short: два"}})
	require.ErrorContains(t, err, "1 results for 2 elements")
}

func TestBackend(t *testing.T) {
	t.Parallel()

	b := azure.New(config.AzureTranslator{}, nil)
	require.Equal(t, "azure", b.Name())
	require.Equal(t, "Azure Translator", b.DisplayName())
	require.Equal(t, translate.Limits{Segments: 1000, Chars: 50000}, b.Limits())

	tr, err := translate.New(config.Translation{Provider: "azure"})
	require.NoError(t, err)
	require.Equal(t, "azure", tr.Name(), "importing the package registers it")
}
