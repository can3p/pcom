package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

// subscribeAs posts the subscribe form of author's journal as the form's own
// page carries it, with the challenge solved; change edits the fields first.
func subscribeAs(t *testing.T, c *e2e.Client, author, email string, change func(url.Values)) *e2e.Response {
	t.Helper()

	token, ok := c.Get("/users/" + author).RequireStatus(http.StatusOK).Doc().
		Find(`[data-testid="subscribe-form"] input[name="pow_challenge"]`).Attr("value")
	require.True(t, ok, "the journal shows the form")

	values := url.Values{
		"email":                 {email},
		abuse.PowChallengeField: {token},
		abuse.PowNonceField:     {e2e.SolveChallenge(t, token)},
	}
	if change != nil {
		change(values)
	}

	return c.PostForm("/form/subscribe/"+author, values)
}

// mailLink is the first link in m matching pattern (a path, as the app's
// site root precedes it).
func mailLink(t *testing.T, app *e2e.App, m tommy.Mail, pattern string) string {
	t.Helper()

	found := regexp.MustCompile(regexp.QuoteMeta(app.URL) + pattern).FindString(m.HTML + m.Text)
	require.NotEmpty(t, found, "%s in the mail", pattern)

	return strings.TrimPrefix(found, app.URL)
}

// subscriptionsOf is the status of every subscription of an address.
func subscriptionsOf(t *testing.T, app *e2e.App, email string) []model.SubscriptionStatus {
	t.Helper()

	ctx := context.Background()

	subscriber, err := factory.GetEmailSubscriber(ctx, app.DB, email)
	if err != nil {
		return nil
	}

	subs, err := factory.ListEmailSubscriptions(ctx, app.DB, subscriber.ID)
	require.NoError(t, err)

	var out []model.SubscriptionStatus
	for _, s := range subs {
		out = append(out, s.Status)
	}

	return out
}

// A visitor subscribes, opens the mailed link (which changes nothing: mail
// scanners open links), confirms, and the author is told.
func TestSubscriptions_SubscribeAndConfirm(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	author := newUser(t, app, factory.Subscribable())
	email := uniqueEmail("reader")
	visitor := app.Client(t)

	resp := subscribeAs(t, visitor, author.Username, email, nil).RequireStatus(http.StatusOK)
	require.Contains(t, resp.Body, "Check your inbox")

	confirm := mailLink(t, app, app.Mails(t, email, nil)[0], `/subscriptions/confirm/[\w-]+`)

	page := visitor.Get(confirm).RequireStatus(http.StatusOK)
	require.Contains(t, page.Body, "Confirm subscription")
	require.Contains(t, page.Body, "@"+author.Username+" will see your email address")
	require.Equal(t, []model.SubscriptionStatus{model.SubscriptionStatusPending}, subscriptionsOf(t, app, email))
	app.NoMails(t, author.Email, func(m tommy.Mail) bool { return strings.Contains(m.Text, email) })

	require.Contains(t, visitor.PostForm(confirm, nil).RequireStatus(http.StatusOK).Body, "You are subscribed")
	require.Equal(t, []model.SubscriptionStatus{model.SubscriptionStatusActive}, subscriptionsOf(t, app, email))
	app.Mails(t, author.Email, func(m tommy.Mail) bool { return strings.Contains(m.Text, email) })

	expired := visitor.Get("/subscriptions/confirm/no-such-token").RequireStatus(http.StatusOK)
	require.Contains(t, expired.Body, "This link has expired")
	require.Contains(t, expired.Body, "Subscribe again from the author")
}

// A person and a script get the same reply, and so do every state of the
// address; only a person's new address is mailed. The same goes for the
// form that mails the manage link.
func TestSubscriptions_SameReplyForEveryState(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	author := newUser(t, app, factory.Subscribable())
	fresh, active, pending, bot := uniqueEmail("new"), uniqueEmail("active"), uniqueEmail("pending"), uniqueEmail("bot")
	testutil.Must(factory.EmailSubscription(ctx, app.DB, author.ID, active))(t)
	testutil.Must(factory.EmailSubscription(ctx, app.DB, author.ID, pending, factory.PendingSince(time.Now())))(t)

	visitor := app.Client(t)
	want := subscribeAs(t, visitor, author.Username, fresh, nil).RequireStatus(http.StatusOK).Body
	require.Contains(t, want, "Check your inbox")

	for _, email := range []string{active, pending} {
		require.Equal(t, want, subscribeAs(t, visitor, author.Username, email, nil).RequireStatus(http.StatusOK).Body, email)
	}

	robot := subscribeAs(t, visitor, author.Username, bot, func(v url.Values) { v.Set(abuse.HoneypotField, "http://spam.test") })
	require.Equal(t, want, robot.RequireStatus(http.StatusOK).Body)

	unsolved := subscribeAs(t, visitor, author.Username, bot, func(v url.Values) { v.Set(abuse.PowNonceField, "") })
	require.Contains(t, unsolved.RequireStatus(http.StatusOK).Body, "please try again")

	app.Mails(t, fresh, nil)
	app.NoMails(t, bot, nil)
	app.NoMails(t, active, nil)
	require.Empty(t, subscriptionsOf(t, app, bot))

	// the lost link form: mailed for an address that subscribes, nothing for one that doesn't
	lost := func(email string) string {
		token, ok := visitor.Get("/subscriptions").RequireStatus(http.StatusOK).Doc().
			Find(`[data-testid="lost-form"] input[name="pow_challenge"]`).Attr("value")
		require.True(t, ok)

		return visitor.PostForm("/form/subscriptions/lost", url.Values{
			"email": {email}, abuse.PowChallengeField: {token}, abuse.PowNonceField: {e2e.SolveChallenge(t, token)},
		}).RequireStatus(http.StatusOK).Body
	}

	stranger := uniqueEmail("stranger")
	require.Equal(t, lost(stranger), lost(active))
	require.Contains(t, lost(active), "If that address subscribes")
	app.Mails(t, active, func(m tommy.Mail) bool { return strings.Contains(m.Text, app.URL+"/subscriptions/") })
	app.NoMails(t, stranger, nil)
}

// A public post reaches an active subscriber, with a one-click unsubscribe
// link that works without a session or a CSRF token; the author is told, and
// a second click finds nothing to do.
func TestSubscriptions_PostMailAndOneClickUnsubscribe(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	author := newUser(t, app, factory.Subscribable())
	email := uniqueEmail("reader")
	testutil.Must(factory.EmailSubscription(ctx, app.DB, author.ID, email))(t)
	key := testutil.Must(factory.APIKey(ctx, app.DB, author.ID))(t)

	body, err := json.Marshal(map[string]any{"subject": "Hello subscribers", "md_body": "A public post", "visibility": "public", "is_published": true})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, app.URL+"/api/v1/posts", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.APIKey)
	req.Header.Set("Content-Type", "application/json")
	app.Client(t).Do(req).RequireStatus(http.StatusOK)

	unsubscribe := mailLink(t, app, app.Mails(t, email, func(m tommy.Mail) bool { return strings.Contains(m.Subject, "Hello subscribers") })[0], `/subscriptions/[\w-]+/unsubscribe/[\w-]+`)

	// the footer link opens a page in a browser and changes nothing
	mailClient := app.Client(t)
	require.Contains(t, mailClient.Get(unsubscribe).RequireStatus(http.StatusOK).Body, author.Username)
	require.Equal(t, []model.SubscriptionStatus{model.SubscriptionStatusActive}, subscriptionsOf(t, app, email))

	// what List-Unsubscribe-Post posts: no session, no CSRF token
	click := func() *e2e.Response {
		post, err := http.NewRequest(http.MethodPost, app.URL+unsubscribe, strings.NewReader("List-Unsubscribe=One-Click"))
		require.NoError(t, err)
		post.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		return app.Client(t).Do(post)
	}

	require.Contains(t, click().RequireStatus(http.StatusOK).Body, "You are unsubscribed")
	require.Empty(t, subscriptionsOf(t, app, email))
	app.Mails(t, author.Email, func(m tommy.Mail) bool { return strings.Contains(m.Text, email) })

	require.Contains(t, click().RequireStatus(http.StatusOK).Body, "This link has expired")
}

// The manage page lists the authors, marks one who pauses subscriptions, and
// unsubscribes from one or from all; a token it does not know is an expired link.
func TestSubscriptions_Manage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	open := newUser(t, app, factory.Subscribable())
	paused := newUser(t, app)
	email := uniqueEmail("reader")
	first := testutil.Must(factory.EmailSubscription(ctx, app.DB, open.ID, email))(t)
	testutil.Must(factory.EmailSubscription(ctx, app.DB, paused.ID, email))(t)

	visitor := app.Client(t)
	manage := "/subscriptions/" + first.Subscriber.ManageToken

	page := visitor.Get(manage).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, 2, page.Find(`[data-testid="subscriptions-list"] li`).Length())
	require.Contains(t, page.Find(`[data-testid="subscription-`+paused.Username+`"]`).Text(), "isn't sending posts by email right now")
	require.NotContains(t, page.Find(`[data-testid="subscription-`+open.Username+`"]`).Text(), "isn't sending")

	one := visitor.PostForm(manage+"/unsubscribe/"+first.ID, nil).RequireStatus(http.StatusOK)
	require.Contains(t, one.Body, "You are unsubscribed")
	require.Len(t, subscriptionsOf(t, app, email), 1)

	all := visitor.PostForm(manage+"/unsubscribe", nil).RequireStatus(http.StatusOK)
	require.Contains(t, all.Body, "You are unsubscribed")
	require.Empty(t, subscriptionsOf(t, app, email))

	require.Contains(t, visitor.Get(manage).RequireStatus(http.StatusOK).Body, "This link has expired")
}

// An author removes a subscriber, silently, and only their own.
func TestSubscriptions_AuthorRemovesSubscriber(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	author := newUser(t, app, factory.Subscribable())
	authorClient := loginAs(t, app, author)
	other := newUser(t, app, factory.Subscribable())

	mine, theirs := uniqueEmail("mine"), uniqueEmail("theirs")
	own := testutil.Must(factory.EmailSubscription(ctx, app.DB, author.ID, mine))(t)
	foreign := testutil.Must(factory.EmailSubscription(ctx, app.DB, other.ID, theirs))(t)

	authorClient.PostJSON("/controls/action/subscribers/"+foreign.ID+"/remove", map[string]string{}).RequireStatus(http.StatusBadRequest)
	require.Len(t, subscriptionsOf(t, app, theirs), 1)

	authorClient.PostJSON("/controls/action/subscribers/"+own.ID+"/remove", map[string]string{}).RequireStatus(http.StatusOK)
	require.Empty(t, subscriptionsOf(t, app, mine))
	app.NoMails(t, author.Email, func(m tommy.Mail) bool { return strings.Contains(m.Text, mine) })
	app.NoMails(t, mine, nil)
}
