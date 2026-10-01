package forms_test

import (
	"context"
	gogoforms "github.com/can3p/gogo/forms"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const (
	testSalt     = "test-salt"
	testSiteRoot = "https://site.test"
)

// codeAccounts is the accounts service with the code key logging in needs.
func codeAccounts(db *sqlx.DB, snd *fakesender.Sender) *accounts.Service {
	return accounts.New(repo.New(db), snd, nil, accounts.WithCodeKey("test-key"))
}

func TestLoginForm_ValidateNeedsAnEmail(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
	form := forms.LoginFormNew(nil, testSalt).(*forms.LoginForm)
	form.Input.Email = "  "

	require.ErrorIs(t, form.Validate(c), gogoforms.ErrValidationFailed)
	require.True(t, form.Errors.HasError("email"))
}

// A known and an unknown address are answered the same way, with the code
// form, and only the known one is mailed.
func TestLoginForm_SaveAnswersAnyAddressTheSame(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	user := testutil.Must(factory.User(context.Background(), db))(t)
	snd := fakesender.New()
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{"link": func(...string) string { return "/x" }}).
		ParseFiles("../../cmd/web/client/html/form--login-code.html"))

	answer := func(email string) (int, string, bool) {
		gin.SetMode(gin.TestMode)

		form := forms.LoginFormNew(codeAccounts(db, snd), testSalt).(*forms.LoginForm)
		r := gin.New()
		r.Use(sessions.Sessions("sess", cookie.NewStore([]byte("secret"))))
		r.SetHTMLTemplate(tmpl)
		r.POST("/login", func(c *gin.Context) {
			gogoforms.DefaultHandler(c, form)
			require.NotEmpty(t, auth.LoginAttempt(c), email)
		})

		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"email": {email}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		mailed := false
		for _, m := range snd.Sent() {
			mailed = mailed || m.Mail.To[0].Address == email
		}

		return w.Code, strings.ReplaceAll(w.Body.String(), email, ""), mailed
	}

	knownCode, knownBody, knownMailed := answer(user.Email)
	unknownCode, unknownBody, unknownMailed := answer("nobody@example.test")

	require.Equal(t, knownCode, unknownCode)
	require.Equal(t, knownBody, unknownBody)
	require.Contains(t, knownBody, `name="code"`)
	require.True(t, knownMailed)
	require.False(t, unknownMailed)
}
