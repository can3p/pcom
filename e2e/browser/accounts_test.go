//go:build browser

package browser_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/google/uuid"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b7SignupCodeRE matches the code in the plain-text body of the signup mail.
var b7SignupCodeRE = regexp.MustCompile(`confirmation code is (\d{8})`)

// b7SignupCode waits for the signup mail this app delivered to email and
// pulls the code out of its plain-text body.
func b7SignupCode(t testing.TB, app *e2e.App, email string) string {
	t.Helper()

	mails := app.Mails(t, email, func(m tommy.Mail) bool { return b7SignupCodeRE.MatchString(m.Text) })
	require.Len(t, mails, 1)

	return b7SignupCodeRE.FindStringSubmatch(mails[0].Text)[1]
}

// A user enters their email, reads the code from the mail and types it: they
// land on their feed.
func TestAccounts_LoginWithEmailedCode(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app)

	_, err := page.Goto("/login")
	require.NoError(t, err)

	browser.LogInWithCode(t, app, page, user.Email)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
}

// A wrong code comes back as an in-place error: the code form is swapped for
// itself with the error shown, the page never navigates away.
func TestAccounts_LoginWrongCodeShowsErrorInPlace(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app)

	_, err := page.Goto("/login")
	require.NoError(t, err)

	browser.SubmitLoginEmail(t, page, user.Email)
	wrong := "00000000"
	if browser.LoginCode(t, app, user.Email) == wrong {
		wrong = "11111111"
	}

	browser.SubmitLoginCode(t, page, wrong)

	require.NoError(t, browser.Expect.Locator(page.Locator(".invalid-feedback")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Code")).ToBeVisible())
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

	browser.LogInWithCode(t, app, page, user.Email)

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
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Login", Exact: new(true)})).ToBeVisible())
}

// Signing up while registration is open mails a code: typing it confirms the
// address and logs the new account in, with no password anywhere.
func TestAccounts_SignupWhileOpenAndConfirmWithCode(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	require.NoError(t, factory.SetRegistrationOpen(context.Background(), app.DB, true))

	// unique, since every test binary run shares one tommy
	email := "b7signup-" + uuid.NewString() + "@example.test"
	const username = "b7signupuser"

	page := browser.Page(t, app)

	_, err := page.Goto("/signup")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "New Account"})).ToBeVisible())
	require.Zero(t, mustCount(t, page.GetByLabel("Password")))

	require.NoError(t, page.GetByLabel("Email address").Fill(email))
	require.NoError(t, page.GetByLabel("Username").Fill(username))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create an account"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Code")).ToBeVisible())

	browser.SubmitLoginCode(t, page, b7SignupCode(t, app, email))

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation")).ToContainText("Hi "+username))
}

// Someone who signs up and never types the signup code is not locked out:
// logging in later with the same address mails a code that confirms it.
func TestAccounts_SignupWithoutTheCodeLogsInLater(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	require.NoError(t, factory.SetRegistrationOpen(context.Background(), app.DB, true))

	// unique, since every test binary run shares one tommy
	email := "b7late-" + uuid.NewString() + "@example.test"
	const username = "b7lateuser"

	page := browser.Page(t, app)

	_, err := page.Goto("/signup")
	require.NoError(t, err)

	require.NoError(t, page.GetByLabel("Email address").Fill(email))
	require.NoError(t, page.GetByLabel("Username").Fill(username))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create an account"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Code")).ToBeVisible())

	_, err = page.Goto("/login")
	require.NoError(t, err)

	browser.LogInWithCode(t, app, page, email)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation")).ToContainText("Hi "+username))
}

func mustCount(t testing.TB, l playwright.Locator) int {
	t.Helper()

	n, err := l.Count()
	require.NoError(t, err)

	return n
}

// An invited person picks a username, reads the code from the mail and types
// it: they land logged in, connected to the inviter.
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
	require.Zero(t, mustCount(t, page.GetByLabel("Password")))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create an account"}).Click())

	browser.SubmitLoginCode(t, page, browser.LoginCode(t, app, "b7invitee@example.test"))

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/feed$`)))

	_, err = page.Goto("/controls")
	require.NoError(t, err)

	// logged in as the new account
	require.NoError(t, browser.Expect.Locator(page.GetByRole("navigation")).ToContainText("Hi "+username, playwright.LocatorAssertionsToContainTextOptions{}))

	// connected to the inviter, listed under the new account's direct connections
	require.NoError(t, browser.Expect.Locator(page.GetByRole("link", playwright.PageGetByRoleOptions{Name: inviter.Username, Exact: new(true)})).ToBeVisible())
}
