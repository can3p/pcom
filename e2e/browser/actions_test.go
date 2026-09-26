//go:build browser

package browser_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// acceptDialogs makes every JS confirm()/prompt() dialog on page succeed,
// which is what the generic action controller needs before it posts.
func acceptDialogs(page playwright.Page) {
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept("browser test note") })
}

// The three-user connection story: A and B are connected, B and C are
// connected, so A and C are second-degree connections through B. A asks for
// an introduction, B signs it, C accepts it, and A ends up directly
// connected to C and can see C's direct-only post.
func TestActions_ConnectionStory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	a := browser.NewUser(t, app)
	b := browser.NewUser(t, app)
	c := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, a.ID, b.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, b.ID, c.ID)
	require.NoError(t, err)

	post, err := factory.Post(ctx, app.DB, c.ID, factory.Published())
	require.NoError(t, err)

	// A asks for an introduction to C.
	pageA := browser.Page(t, app, browser.As(a))
	acceptDialogs(pageA)

	_, err = pageA.Goto("/users/" + c.Username)
	require.NoError(t, err)

	require.NoError(t, pageA.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Ask for introduction"}).Click())
	require.NoError(t, browser.Expect.Locator(pageA.GetByText("You've requested mediation request")).ToBeVisible())

	// B signs the introduction.
	pageB := browser.Page(t, app, browser.As(b))
	acceptDialogs(pageB)

	_, err = pageB.Goto("/controls")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(pageB.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToBeVisible())
	require.NoError(t, pageB.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign"}).Click())
	require.NoError(t, browser.Expect.Locator(pageB.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToHaveCount(0))

	// C accepts the connection request.
	pageC := browser.Page(t, app, browser.As(c))
	acceptDialogs(pageC)

	_, err = pageC.Goto("/controls")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(pageC.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Connection Requests", Exact: playwright.Bool(true)})).ToBeVisible())
	require.NoError(t, pageC.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Accept"}).Click())
	require.NoError(t, browser.Expect.Locator(pageC.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Connection Requests", Exact: playwright.Bool(true)})).ToHaveCount(0))

	exists, err := factory.ConnectionExists(ctx, app.DB, a.ID, c.ID)
	require.NoError(t, err)
	require.True(t, exists)

	// A now sees C's direct-only post.
	_, err = pageA.Goto("/users/" + c.Username)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(pageA.GetByText("You're connected with "+c.Username)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(pageA.GetByRole("link", playwright.PageGetByRoleOptions{Name: post.Subject.String})).ToBeVisible())
}

// Whitelisting lets a user connect without mediation; removing the
// whitelist entry, connecting and dropping the connection are each
// exercised once.
func TestActions_WhitelistAndDirectConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	a := browser.NewUser(t, app)
	b := browser.NewUser(t, app)

	pageB := browser.Page(t, app, browser.As(b))
	acceptDialogs(pageB)

	_, err := pageB.Goto("/controls")
	require.NoError(t, err)

	whitelistRow := pageB.GetByRole("listitem").Filter(playwright.LocatorFilterOptions{HasText: a.Username})

	require.NoError(t, pageB.Locator("#newPostUsername").Fill(a.Username))
	require.NoError(t, pageB.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add"}).Click())
	require.NoError(t, browser.Expect.Locator(whitelistRow).ToHaveCount(1))

	// remove_from_whitelist
	require.NoError(t, whitelistRow.GetByRole("button").Click())
	require.NoError(t, browser.Expect.Locator(whitelistRow).ToHaveCount(0))

	// whitelist again so A is allowed to connect
	require.NoError(t, pageB.Locator("#newPostUsername").Fill(a.Username))
	require.NoError(t, pageB.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add"}).Click())
	require.NoError(t, browser.Expect.Locator(whitelistRow).ToHaveCount(1))

	pageA := browser.Page(t, app, browser.As(a))
	acceptDialogs(pageA)

	_, err = pageA.Goto("/users/" + b.Username)
	require.NoError(t, err)

	// create_connection
	require.NoError(t, pageA.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Connect"}).Click())
	require.NoError(t, browser.Expect.Locator(pageA.GetByText("You're connected with "+b.Username)).ToBeVisible())

	exists, err := factory.ConnectionExists(ctx, app.DB, a.ID, b.ID)
	require.NoError(t, err)
	require.True(t, exists)

	// drop_connection
	require.NoError(t, pageA.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Disconect"}).Click())
	require.NoError(t, browser.Expect.Locator(pageA.GetByText("You have no relation to "+b.Username)).ToBeVisible())

	exists, err = factory.ConnectionExists(ctx, app.DB, a.ID, b.ID)
	require.NoError(t, err)
	require.False(t, exists)
}

// A mediation request can be revoked by the user who asked for it.
func TestActions_MediationRevoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	x := browser.NewUser(t, app)
	y := browser.NewUser(t, app)
	z := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, x.ID, y.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, y.ID, z.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(x))
	acceptDialogs(page)

	_, err = page.Goto("/users/" + z.Username)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Ask for introduction"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Revoke request"})).ToBeVisible())

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Revoke request"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Ask for introduction"})).ToBeVisible())
}

// A mediator can dismiss a mediation request instead of signing it.
func TestActions_MediationDismiss(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	x := browser.NewUser(t, app)
	y := browser.NewUser(t, app)
	z := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, x.ID, y.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, y.ID, z.ID)
	require.NoError(t, err)

	pageX := browser.Page(t, app, browser.As(x))
	acceptDialogs(pageX)

	_, err = pageX.Goto("/users/" + z.Username)
	require.NoError(t, err)
	require.NoError(t, pageX.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Ask for introduction"}).Click())
	require.NoError(t, browser.Expect.Locator(pageX.GetByText("You've requested mediation request")).ToBeVisible())

	pageY := browser.Page(t, app, browser.As(y))
	acceptDialogs(pageY)

	_, err = pageY.Goto("/controls")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(pageY.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToBeVisible())
	require.NoError(t, pageY.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dismiss"}).Click())
	require.NoError(t, browser.Expect.Locator(pageY.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToHaveCount(0))
}

// The target of a connection request can reject it even after a mediator
// has signed it.
func TestActions_ConnectionRequestReject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	x := browser.NewUser(t, app)
	y := browser.NewUser(t, app)
	z := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, x.ID, y.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, y.ID, z.ID)
	require.NoError(t, err)

	pageX := browser.Page(t, app, browser.As(x))
	acceptDialogs(pageX)

	_, err = pageX.Goto("/users/" + z.Username)
	require.NoError(t, err)
	require.NoError(t, pageX.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Ask for introduction"}).Click())
	require.NoError(t, browser.Expect.Locator(pageX.GetByText("You've requested mediation request")).ToBeVisible())

	pageY := browser.Page(t, app, browser.As(y))
	acceptDialogs(pageY)

	_, err = pageY.Goto("/controls")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(pageY.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToBeVisible())
	require.NoError(t, pageY.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign"}).Click())
	require.NoError(t, browser.Expect.Locator(pageY.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Mediation Requests", Exact: playwright.Bool(true)})).ToHaveCount(0))

	pageZ := browser.Page(t, app, browser.As(z))
	acceptDialogs(pageZ)

	_, err = pageZ.Goto("/controls")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(pageZ.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Connection Requests", Exact: playwright.Bool(true)})).ToBeVisible())
	require.NoError(t, pageZ.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Reject"}).Click())
	require.NoError(t, browser.Expect.Locator(pageZ.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Connection Requests", Exact: playwright.Bool(true)})).ToHaveCount(0))

	exists, err := factory.ConnectionExists(ctx, app.DB, x.ID, z.ID)
	require.NoError(t, err)
	require.False(t, exists)
}

// Creating a share exposes a public link that opens anonymously; deleting the share makes it disappear
// from the post page and the link stop working.
func TestActions_ShareLifecycle(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())

	author := browser.NewUser(t, app)
	post, err := factory.Post(context.Background(), app.DB, author.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(author), browser.Allow(`404`))
	acceptDialogs(page)

	_, err = page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	// create_share
	require.NoError(t, page.Locator(".us-comment-stats a:has(i.bi-share)").Click())

	shareLink := page.Locator(`.us-public-link a[href^="/shared/"]`)
	require.NoError(t, browser.Expect.Locator(shareLink).ToBeVisible())

	href, err := shareLink.GetAttribute("href")
	require.NoError(t, err)
	require.NotEmpty(t, href)

	// open the share anonymously
	anon := browser.Page(t, app, browser.Allow(`404`))

	_, err = anon.Goto(href)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(anon.GetByRole("heading", playwright.PageGetByRoleOptions{Name: post.Subject.String})).ToBeVisible())

	// delete_share
	require.NoError(t, page.Locator(".us-public-link a:has(i.bi-trash)").Click())
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-public-link")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-comment-stats a:has(i.bi-share)")).ToBeVisible())

	resp, err := anon.Goto(href)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 404, resp.Status())
}

// Asking a direct connection for a post and dismissing an incoming prompt.
func TestActions_PostPromptAskAndDismiss(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())

	asker := browser.NewUser(t, app)
	recipient := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, asker.ID, recipient.ID)
	require.NoError(t, err)

	pageAsker := browser.Page(t, app, browser.As(asker))

	_, err = pageAsker.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, pageAsker.GetByPlaceholder("Ask for a post: What's up?").Fill("What made you smile today?"))
	require.NoError(t, pageAsker.GetByPlaceholder("%username%").Fill(recipient.Username))
	require.NoError(t, pageAsker.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Prompt!"}).Click())

	require.NoError(t, browser.Expect.Locator(pageAsker.GetByRole("alert")).ToContainText("Prompt has been sent"))

	pageRecipient := browser.Page(t, app, browser.As(recipient))
	acceptDialogs(pageRecipient)

	_, err = pageRecipient.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(pageRecipient.GetByText("What made you smile today?")).ToBeVisible())

	// dismiss_prompt
	require.NoError(t, pageRecipient.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dismiss"}).Click())
	require.NoError(t, browser.Expect.Locator(pageRecipient.GetByText("What made you smile today?")).ToHaveCount(0))
}
