//go:build browser

package browser_test

import (
	"testing"

	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
)

func TestMain(m *testing.M) {
	wiremock.Register("testdata/wiremock")
	browser.Main(m)
}
