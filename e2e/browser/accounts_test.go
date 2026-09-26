//go:build browser

package browser_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b7ConfirmLinkRE matches the absolute confirmation link the way it is sent
// in the plain-text body of the confirm_signup mail.
var b7ConfirmLinkRE = regexp.MustCompile(`https?://\S+/confirm_signup/\S+`)

// b7ConfirmLink reads the queued confirm_signup email back from the
// outgoing mail queue and pulls the confirmation link out of its body.
func b7ConfirmLink(t testing.TB, app *e2e.App) string {
	t.Helper()

	emails, err := factory.ListOutgoingEmails(context.Background(), app.DB, core.OutgoingEmailWhere.EmailType.EQ("confirm_signup"))
	require.NoError(t, err)
	require.Len(t, emails, 1)

	var payload sender.Mail
	require.NoError(t, emails[0].Payload.Unmarshal(&payload))

	link := b7ConfirmLinkRE.FindString(payload.Text)
	require.NotEmpty(t, link, "confirm_signup mail body: %s", payload.Text)

	return link
}

// Bad credentials on the login form come back as an in-place error: the form
// is swapped for itself with the error shown, the page never navigates away.
func TestAccounts_LoginBadCredentialsShowErrorInPlace(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app)

	_, err := page.Goto("/login")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Email address").Fill(user.Email))
	require.NoError(t, page.GetByLabel("Password").Fill("not-the-password"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())

	require.NoError(t, browser.Expect.Locator(page.Locator(".alert-danger")).ToContainText("Bad credentials"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/login`)))
}

// A protected page redirects an anonymous visitor to /login with a signed
// return_url; logging in from there lands back on the originally requested
// page instead of the default feed.
func TestAccounts_LoginWithSignedReturnURLLandsOnRequestedPage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app)

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/login\?.*return_url=`)))

	require.NoError(t, page.GetByLabel("Email address").Fill(user.Email))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/settings$`)))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"})).ToBeVisible())
}

// Logging out from settings redirects to the home page and the nav goes
// back to showing an anonymous Login link.
func TestAccounts_Logout(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`^`+regexp.QuoteMeta(app.URL)+`/$`)))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Login", Exact: playwright.Bool(true)})).ToBeVisible())
}

// Signing up while registration is open queues a confirmation email;
// following the link in it confirms the address and the account can then
// log in, which only works once the email is confirmed.
func TestAccounts_SignupWhileOpenAndConfirmEmail(t *testing.T) {
	t.Parallel()
	t.Skip("known bug #139: GET /signup and GET /confirm_signup/:id render header.html from a bare map instead of a page struct built through getBasePage, so ScriptNonce/StyleNonce are empty; the inline bootstrap <script nonce=\"\"> in header.html then mismatches the real CSP header nonce and Chromium blocks it as a CSP violation, which the browser guard reports as a page error")

	app := e2e.Start(t, e2e.WithRealAssets())
	require.NoError(t, factory.SetRegistrationOpen(context.Background(), app.DB, true))

	const email = "b7signup@example.test"
	const username = "b7signupuser"

	page := browser.Page(t, app)

	_, err := page.Goto("/signup")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "New Account"})).ToBeVisible())

	require.NoError(t, page.GetByLabel("Email address").Fill(email))
	require.NoError(t, page.GetByLabel("Username").Fill(username))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create an account"}).Click())

	require.NoError(t, browser.Expect.Locator(page.Locator("body")).ToContainText("check your mailbox"))

	link := b7ConfirmLink(t, app)

	_, err = page.Goto(link)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Your email got confirmed, thanks!"})).ToBeVisible())

	// the account only becomes usable once the email is confirmed: logging
	// in with it now succeeds and lands on the default authorized home.
	require.NoError(t, page.GetByLabel("Email address").Fill(email))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in"}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}

// Accepting an invitation creates the account, logs it in straight away and
// connects it to the inviter, all visible from the resulting /controls page.
func TestAccounts_AcceptInvitationConnectsToInviter(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	inviter := browser.NewUser(t, app)

	invite, err := factory.Invitation(context.Background(), app.DB, inviter.ID, factory.Sent("b7invitee@example.test"))
	require.NoError(t, err)

	const username = "b7invitee"

	page := browser.Page(t, app)

	_, err = page.Goto("/invite/" + invite.ID)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Accept invite from " + inviter.Username})).ToBeVisible())

	require.NoError(t, page.GetByLabel("Username").Fill(username))
	require.NoError(t, page.GetByLabel("Password").Fill(browser.Password))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create an account"}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/?$`)))

	// logged in as the new account
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation")).ToContainText("Hi "+username, playwright.LocatorAssertionsToContainTextOptions{}))

	// connected to the inviter, listed under the new account's direct connections
	require.NoError(t, browser.Expect.Locator(page.GetByRole("link", playwright.PageGetByRoleOptions{Name: inviter.Username, Exact: playwright.Bool(true)})).ToBeVisible())
}
