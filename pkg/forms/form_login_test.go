package forms_test

import (
	"context"
	"database/sql"
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
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testSalt     = "test-salt"
	testSiteRoot = "https://site.test"
)

// testAdmin is the address the admin's mails go to.
const testAdmin = "admin@site.test"

// testIdent is the identity the accounts services under test mail with.
var testIdent = mail.Identity{Site: links.Site{Root: testSiteRoot}, From: "sender@site.test", AdminAddress: testAdmin}

// codeAccounts is the accounts service with the code key logging in needs.
func codeAccounts(db *sql.DB, q *fakequeue.Queue) *accounts.Service {
	if q == nil {
		q = fakequeue.New()
	}

	return accounts.New(repo.New(db), nil, nil, accounts.WithCodeKey("test-key"), accounts.WithIdentity(testIdent), accounts.WithJobs(q))
}

// rendered builds the mail of every account job q recorded, in order, the way
// the mail workers do when they run.
func rendered(t *testing.T, ctx context.Context, svc *accounts.Service, q *fakequeue.Queue) []*mail.Envelope {
	t.Helper()

	var out []*mail.Envelope

	for _, args := range q.Inserted() {
		var (
			env *mail.Envelope
			err error
		)

		switch a := args.(type) {
		case accounts.InvitationMailArgs:
			env, err = svc.RenderInvitation(ctx, a)
		case accounts.AdminNewUserMailArgs:
			env, err = svc.RenderAdminNewUser(ctx, a)
		case accounts.ConfirmWaitingListMailArgs:
			env, err = svc.RenderConfirmWaitingList(ctx, a)
		case accounts.AdminNewWaitingListMemberMailArgs:
			env, err = svc.RenderAdminNewWaitingListMember(ctx, a)
		case accounts.LoginCodeMailArgs:
			env, err = svc.RenderLoginCode(ctx, a)
		case accounts.ConfirmSignupMailArgs:
			env, err = svc.RenderConfirmSignup(ctx, a)
		default:
			t.Fatalf("unexpected job %T", args)
		}

		require.NoError(t, err, args.Kind())
		require.NotNil(t, env, args.Kind())

		out = append(out, env)
	}

	return out
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
	snd := fakequeue.New()
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{"link": func(...string) string { return "/x" }}).
		ParseFiles("../../cmd/web/client/html/form--login-code.html"))

	answer := func(email string) (int, string, bool) {
		gin.SetMode(gin.TestMode)

		form := forms.LoginFormNew(codeAccounts(db, snd), testSalt).(*forms.LoginForm)
		r := gin.New()
		r.Use(sessions.Sessions("sess", cookie.NewStore([]byte("secret"))))
		r.SetHTMLTemplate(tmpl)
		r.POST("/login", func(c *gin.Context) {
			gogoforms.DefaultHandler(c, zap.NewNop(), form)
			require.NotEmpty(t, auth.LoginAttempt(c), email)
		})

		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"email": {email}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		mailed := false
		for _, m := range rendered(t, context.Background(), codeAccounts(db, snd), snd) {
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
