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
	require.Equal(t, w.URL("anthropic"), cfg.env["PROOFREAD_ANTHROPIC_ENDPOINT"])
	require.Contains(t, processEnv(cfg.env), "PROOFREAD_ANTHROPIC_KEY="+WireMockAnthropicKey)

	resp, err := http.Get(w.BaseURL + "/__admin/health")
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
