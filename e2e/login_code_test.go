package e2e_test

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

var mailedCodeRE = regexp.MustCompile(`login code is (\d{6})`)

// startLogin posts email to a fresh client and returns it with the code the
// mail to email carries.
func startLogin(t *testing.T, app *e2e.App, email string) (*e2e.Client, string) {
	t.Helper()

	client := app.Client(t)
	client.Get("/login").RequireStatus(http.StatusOK)
	client.PostForm("/form/login", url.Values{"email": {email}}).RequireStatus(http.StatusOK)

	mails := app.Mails(t, email, func(m tommy.Mail) bool { return m.Subject == "Your pcom login code" })
	m := mailedCodeRE.FindStringSubmatch(mails[0].Text)
	require.NotNil(t, m, mails[0].Text)

	return client, m[1]
}

// postCode posts code and reports whether the client can then open /feed.
func postCode(client *e2e.Client, code string) bool {
	client.PostForm("/form/login/code", url.Values{"code": {code}}).RequireStatus(http.StatusOK)

	return client.Get("/feed").StatusCode == http.StatusOK
}

// TestLoginCode_CodeBelongsToItsSessionsAttempt: with two attempts open, a
// code posted from the other session is refused, and from its own it logs in.
func TestLoginCode_CodeBelongsToItsSessionsAttempt(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := newUser(t, app)

	a, codeA := startLogin(t, app, user.Email)
	b, _ := startLogin(t, app, user.Email)

	require.False(t, postCode(b, codeA))
	require.True(t, postCode(a, codeA))
}
