package e2e

import (
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/stretchr/testify/require"
)

// TestWithWireMockStartsTheContainer: the option starts the shared container,
// which answers, and hands the binary's environment the provider and the
// endpoint, exactly as Start passes them to the process.
func TestWithWireMockStartsTheContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}

	cfg := config{env: map[string]string{}}
	WithWireMock()(&cfg)
	cfg.resolve(t)

	w := wiremock.Shared(t)
	require.Equal(t, "azure", cfg.env["TRANSLATION_PROVIDER"])
	require.NotEmpty(t, cfg.env["TRANSLATION_AZURE_KEY"])
	require.Equal(t, w.URL("azure-translator"), cfg.env["TRANSLATION_AZURE_ENDPOINT"])
	require.Contains(t, processEnv(cfg.env), "TRANSLATION_PROVIDER=azure")

	resp, err := http.Get(w.BaseURL + "/__admin/health")
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
