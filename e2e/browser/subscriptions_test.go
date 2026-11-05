//go:build browser

package browser_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/google/uuid"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// A visitor subscribes from a journal: the form solves its proof of work
// itself and is replaced by the reply; a refused address comes back with the
// error and a form that can be sent again; the mailed link's page confirms.
func TestSubscriptions_SubscribeFromJournal(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()
	author := browser.NewUser(t, app, factory.Subscribable())
	off := browser.NewUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	reader := browser.NewUser(t, app)
	email := fmt.Sprintf("reader-%s@example.test", uuid.NewString())

	// whoever has an account follows through connections; a public journal whose author turned subscriptions off shows no form
	member := browser.Page(t, app, browser.As(reader))
	_, err := member.Goto("/users/" + author.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(member.GetByRole("heading", playwright.PageGetByRoleOptions{Name: author.Username})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(member.GetByTestId("subscribe-form")).ToHaveCount(0))

	page := browser.Page(t, app)
	_, err = page.Goto("/users/" + off.Username)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: off.Username})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscribe-form")).ToHaveCount(0))

	_, err = page.Goto("/users/" + author.Username)
	require.NoError(t, err)

	form := page.GetByTestId("subscribe-form")
	require.NoError(t, browser.Expect.Locator(form).ToContainText("@"+author.Username+" will see your email address"))

	require.NoError(t, form.GetByLabel("Email address").Fill("someone@mailinator.com"))
	require.NoError(t, form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Subscribe"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("field-error-email")).ToHaveText(subscriptions.DisposableMessage))

	form = page.GetByTestId("subscribe-form")
	require.NoError(t, form.GetByLabel("Email address").Fill(email))
	require.NoError(t, form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Subscribe"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscribe-sent")).ToContainText("Check your inbox"))
	require.NoError(t, browser.Expect.Locator(form).ToHaveCount(0))

	mails := app.Mails(t, email, nil)
	link := regexp.MustCompile(`/subscriptions/confirm/[\w-]+`).FindString(mails[0].Text)
	require.NotEmpty(t, link)

	_, err = page.Goto(link)
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Confirm subscription"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscription-confirmed")).ToContainText("@"+author.Username))

	subscriber, err := factory.GetEmailSubscriber(ctx, app.DB, email)
	require.NoError(t, err)
	subs, err := factory.ListEmailSubscriptions(ctx, app.DB, subscriber.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, model.SubscriptionStatusActive, subs[0].Status)
}

// The same form is on a public post, and a visitor who lost the link asks for
// a new one from the page that explains it.
func TestSubscriptions_PostPageAndLostLink(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()
	author := browser.NewUser(t, app, factory.Subscribable())
	public := testutil.Must(factory.Post(ctx, app.DB, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic)))(t)
	email := fmt.Sprintf("reader-%s@example.test", uuid.NewString())
	testutil.Must(factory.EmailSubscription(ctx, app.DB, author.ID, email))(t)

	page := browser.Page(t, app)
	_, err := page.Goto("/posts/" + public.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscribe-form")).ToBeVisible())

	_, err = page.Goto("/subscriptions")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Email address").Fill(email))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Mail me the link"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("lost-sent")).ToContainText("If that address subscribes"))

	mails := app.Mails(t, email, nil)
	require.Regexp(t, `/subscriptions/[\w-]+`, mails[0].Text)
}

// The manage page asks before it unsubscribes, from one author or from all,
// and marks an author who pauses subscriptions.
func TestSubscriptions_ManagePage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	ctx := context.Background()
	open := browser.NewUser(t, app, factory.Subscribable())
	paused := browser.NewUser(t, app)
	email := fmt.Sprintf("reader-%s@example.test", uuid.NewString())
	first := testutil.Must(factory.EmailSubscription(ctx, app.DB, open.ID, email))(t)
	testutil.Must(factory.EmailSubscription(ctx, app.DB, paused.ID, email))(t)

	asked := make(chan string, 4)
	accept := false
	page := browser.Page(t, app)
	page.OnDialog(func(d playwright.Dialog) {
		asked <- d.Message()

		if accept {
			_ = d.Accept()
		} else {
			_ = d.Dismiss()
		}
	})

	_, err := page.Goto("/subscriptions/" + first.Subscriber.ManageToken)
	require.NoError(t, err)

	rows := page.GetByTestId("subscriptions-list").GetByRole("listitem")
	require.NoError(t, browser.Expect.Locator(rows).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscription-"+paused.Username)).ToContainText("isn't sending posts by email right now"))

	row := page.GetByTestId("subscription-" + open.Username)
	require.NoError(t, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Unsubscribe"}).Click())
	require.Contains(t, <-asked, "@"+open.Username)

	remaining, err := factory.ListEmailSubscriptions(ctx, app.DB, first.SubscriberID)
	require.NoError(t, err)
	require.Len(t, remaining, 2, "a dismissed question changes nothing")

	accept = true

	require.NoError(t, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Unsubscribe"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscriptions-notice")).ToHaveText("You are unsubscribed."))
	require.NoError(t, browser.Expect.Locator(rows).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscription-"+open.Username)).ToHaveCount(0))

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Unsubscribe from all"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("subscriptions-notice")).ToHaveText("You are unsubscribed."))
	require.NoError(t, browser.Expect.Locator(rows).ToHaveCount(0))

	_, err = factory.GetEmailSubscriber(ctx, app.DB, email)
	require.Error(t, err)
}
