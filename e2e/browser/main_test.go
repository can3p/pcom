//go:build browser

package browser_test

import (
	"testing"

	"github.com/can3p/pcom/e2e/browser"
)

func TestMain(m *testing.M) { browser.Main(m) }
