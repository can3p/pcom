package forms_test

import (
	"context"
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gogoforms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeGuard answers every Verify with err and remembers what it was given.
type fakeGuard struct {
	err  error
	last abuse.Submission
}

func (g *fakeGuard) Issue() abuse.Challenge { return abuse.Challenge{Token: "fresh-challenge"} }

func (g *fakeGuard) Verify(_ context.Context, sub abuse.Submission) error {
	g.last = sub

	return g.err
}

func subscriptionsFor(db *sql.DB, q *fakequeue.Queue) *subscriptions.Service {
	return subscriptions.New(repo.New(db), subscriptions.WithJobs(q), subscriptions.WithEmailChecker(emailaddr.NewChecker(emailaddr.WithTimeout(0))))
}

// answer posts values to the form the way the router does, rendering the real
// templates, and returns the response.
func answer(t *testing.T, form gogoforms.Form, values url.Values) *httptest.ResponseRecorder {
	t.Helper()

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"link":  func(args ...string) string { return "/" + strings.Join(args, "/") },
		"toMap": func(...any) map[string]any { return nil },
	}).ParseFiles("../../cmd/web/client/html/partial--subscribe.html", "../../cmd/web/client/html/subscriptions_lost.html"))

	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.SetHTMLTemplate(tmpl)
	r.POST("/", func(c *gin.Context) { gogoforms.DefaultHandler(c, zap.NewNop(), form) })

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

// human is the fields of a submission the guard let through.
func human(email string) url.Values {
	return url.Values{
		"email":                 {email},
		abuse.PowChallengeField: {"signed-challenge"},
		abuse.PowNonceField:     {"42"},
	}
}

func TestSubscribeForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	private := testutil.Must(factory.User(ctx, db))(t)

	for _, tc := range []struct {
		name      string
		author    string
		email     string
		guardErr  error
		wantCode  int
		wantBody  string // in the response
		wantQueue int    // confirmation mails enqueued
	}{
		{name: "a person subscribes", author: author.Username, email: "reader@example.test", wantCode: http.StatusOK, wantBody: "Check your inbox", wantQueue: 1},
		{name: "a script gets the same reply and no mail", author: author.Username, email: "bot@example.test", guardErr: abuse.ErrSilent, wantCode: http.StatusOK, wantBody: "Check your inbox"},
		{name: "a refused proof of work asks to retry with a fresh challenge", author: author.Username, email: "slow@example.test", guardErr: abuse.ErrInvalid, wantCode: http.StatusOK, wantBody: forms.RetryMessage + `</span>`},
		{name: "a throwaway address is refused on the field", author: author.Username, email: "x@mailinator.com", wantCode: http.StatusOK, wantBody: subscriptions.DisposableMessage},
		{name: "an empty address", author: author.Username, email: " ", wantCode: http.StatusOK, wantBody: "email is required"},
		{name: "an author who takes no subscriptions", author: private.Username, email: "reader2@example.test", wantCode: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := fakequeue.New()
			guard := &fakeGuard{err: tc.guardErr}
			w := answer(t, forms.SubscribeFormNew(subscriptionsFor(db, q), guard, tc.author), human(tc.email))

			require.Equal(t, tc.wantCode, w.Code)
			require.Contains(t, w.Body.String(), tc.wantBody)
			require.Len(t, fakequeue.Of[subscriptions.ConfirmMailArgs](q), tc.wantQueue)

			if tc.guardErr != nil && !errors.Is(tc.guardErr, abuse.ErrSilent) {
				require.Contains(t, w.Body.String(), `value="fresh-challenge"`, "a form that came back carries a new challenge")
			}
		})
	}
}

// What the guard is given is what the form's fields carried, under the names
// the abuse package exports.
func TestSubscribeForm_PassesTheGuardsFieldsOn(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	author := testutil.Must(factory.User(context.Background(), db, factory.Subscribable()))(t)
	guard := &fakeGuard{}

	values := human("reader@example.test")
	values.Set(abuse.HoneypotField, "http://spam.test")
	answer(t, forms.SubscribeFormNew(subscriptionsFor(db, fakequeue.New()), guard, author.Username), values)

	require.Equal(t, abuse.Submission{Token: "signed-challenge", Nonce: "42", Honeypot: "http://spam.test"}, guard.last)
}

// The reply is the same for every state of the address.
func TestSubscribeForm_SameReplyForEveryState(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "active@example.test"))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "pending@example.test", factory.PendingSince(time.Now())))(t)

	var want string

	for _, email := range []string{"new@example.test", "active@example.test", "pending@example.test"} {
		w := answer(t, forms.SubscribeFormNew(subscriptionsFor(db, fakequeue.New()), &fakeGuard{}, author.Username), human(email))
		require.Equal(t, http.StatusOK, w.Code)

		if want == "" {
			want = w.Body.String()
		}

		require.Equal(t, want, w.Body.String(), email)
	}
}

func TestSubscriptionsLostForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	author := testutil.Must(factory.User(ctx, db, factory.Subscribable()))(t)
	testutil.Must(factory.EmailSubscription(ctx, db, author.ID, "known@example.test"))(t)

	for _, tc := range []struct {
		name      string
		email     string
		guardErr  error
		wantBody  string
		wantQueue int
	}{
		{name: "an address that subscribes is mailed", email: "known@example.test", wantBody: "If that address subscribes", wantQueue: 1},
		{name: "an unknown address gets the same reply and no mail", email: "unknown@example.test", wantBody: "If that address subscribes"},
		{name: "a script gets the same reply and no mail", email: "known@example.test", guardErr: abuse.ErrSilent, wantBody: "If that address subscribes"},
		{name: "a refused proof of work asks to retry", email: "known@example.test", guardErr: abuse.ErrInvalid, wantBody: forms.RetryMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q := fakequeue.New()
			w := answer(t, forms.SubscriptionsLostFormNew(subscriptionsFor(db, q), &fakeGuard{err: tc.guardErr}), human(tc.email))

			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), tc.wantBody)
			require.Len(t, fakequeue.Of[subscriptions.ManageLinkMailArgs](q), tc.wantQueue)
		})
	}
}
