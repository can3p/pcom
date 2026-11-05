//go:build browser

// The proof-of-work controller: a form that carries it solves the challenge
// before it is sent, at the default difficulty, in a time a reader accepts
// even on a slow phone.
package browser_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"strconv"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

const (
	// A mid-range phone is about four times slower than a desktop. The
	// default difficulty is set so the average challenge takes about a second
	// throttled; the budget leaves room for slow CI machines.
	powCPUThrottle = 4
	powBudget      = 5 * time.Second
)

// averageChallenge issues challenges until one needs about the expected
// number of hashes (2^difficulty), so the time the test measures is the
// controller's speed and not the luck of one draw.
func averageChallenge(g *abuse.Guard) abuse.Challenge {
	expected := 1 << g.Difficulty()

	for {
		c := g.Issue()

		for n := 0; n < 2*expected; n++ {
			sum := sha256.Sum256([]byte(c.Token + strconv.Itoa(n)))
			if bits.LeadingZeros32(binary.BigEndian.Uint32(sum[:4])) >= g.Difficulty() {
				if n > expected*3/4 && n < expected*5/4 {
					return c
				}

				break
			}
		}
	}
}

// No page carries such a form yet, so the test injects one: Stimulus
// connects it as it would a form the server rendered.
func TestPow_SolvesTheChallengeAtDefaultDifficulty(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	_, err := page.Goto("/feed")
	require.NoError(t, err)

	cdp, err := page.Context().NewCDPSession(page)
	require.NoError(t, err)
	_, err = cdp.Send("Emulation.setCPUThrottlingRate", map[string]any{"rate": powCPUThrottle})
	require.NoError(t, err)

	// The fill-time check is the service's test; here the form may come back at once.
	guard := abuse.New(e2e.SessionSalt, abuse.WithMinFillTime(time.Nanosecond))
	challenge := averageChallenge(guard)

	_, err = page.Evaluate(`token => {
		const form = document.createElement("form")
		form.id = "pow-form"
		form.dataset.controller = "pow"
		form.dataset.powChallengeValue = token
		form.innerHTML = '<input type="hidden" name="pow_nonce" data-pow-target="nonce"><button type="submit">Subscribe</button>'
		// Stands in for the server: it records the nonce the form sends.
		form.addEventListener("submit", (e) => {
			e.preventDefault()
			window.sent = form.elements.pow_nonce.value
		})
		document.querySelector("main, body").append(form)
	}`, challenge.Token)
	require.NoError(t, err)

	button := page.Locator("#pow-form button")
	start := time.Now()
	require.NoError(t, button.Click())
	require.NoError(t, browser.Expect.Locator(button).ToHaveText("Checking…"))

	_, err = page.WaitForFunction(`() => window.sent !== undefined`, nil,
		playwright.PageWaitForFunctionOptions{Timeout: new(float64(powBudget.Milliseconds()))})
	require.NoError(t, err, "the challenge was not solved within %s at %dx CPU throttling", powBudget, powCPUThrottle)
	t.Logf("solved at %d bits in %s with %dx CPU throttling", guard.Difficulty(), time.Since(start), powCPUThrottle)

	require.NoError(t, browser.Expect.Locator(button).ToHaveText("Subscribe"))

	nonce, err := page.Evaluate(`window.sent`)
	require.NoError(t, err)
	require.NotEmpty(t, nonce)

	// What the browser hashed is what the server verifies.
	require.NoError(t, guard.Verify(context.Background(), repo.New(app.DB), abuse.Submission{
		Token: challenge.Token,
		Nonce: nonce.(string),
	}))
}
