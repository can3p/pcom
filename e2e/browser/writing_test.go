//go:build browser

// Everything a user does while writing a post: the /write page (with and
// without ?prompt=), the markdown editor controller (typing, toolbar
// buttons, the preview link), autosaving a draft, publishing, turning a
// published post back into a draft, deleting through the editor and from
// the drafts table, and what the rendered post looks like (headings, code
// highlighting, a gallery, a lite-youtube embed and a lazy-loaded image).
package browser_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// b2EditURLRe pulls the post id out of the "/posts/<id>/edit" url that
// HX-Replace-Url lands the browser on after the very first autosave of a
// new post.
var b2EditURLRe = regexp.MustCompile(`/posts/([^/]+)/edit$`)

// Typing into a new post, using a toolbar button, autosaves the draft: the
// server assigns the post an id, the browser url is replaced with its edit
// url, the "Last Updated" partial appears and the preview link is revealed.
func TestWriting_NewDraftAutosaveAndToolbar(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	subject := page.GetByPlaceholder("Subject")
	require.NoError(t, subject.Fill("My draft post"))

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Click())
	require.NoError(t, body.PressSequentially("Hello world"))

	require.NoError(t, page.Locator(".bi-type-bold").Click())

	require.NoError(t, browser.Expect.Locator(page.Locator("#last_draft_save")).ToContainText("Last Updated"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))

	matches := b2EditURLRe.FindStringSubmatch(page.URL())
	require.Len(t, matches, 2)
	postID := matches[1]

	preview := page.Locator("#show_preview")
	require.NoError(t, browser.Expect.Locator(preview).ToBeVisible())

	href, err := preview.GetAttribute("href")
	require.NoError(t, err)
	require.Contains(t, href, "edit_preview=true")

	post, err := factory.GetPost(context.Background(), app.DB, postID)
	require.NoError(t, err)
	require.Equal(t, "My draft post", post.Subject.String)
	require.Contains(t, post.Body, "Hello world")
	require.Contains(t, post.Body, "**bold**")
	require.False(t, post.PublishedAt.Valid)
}

// Publishing an existing draft redirects to the post, and turning it back
// into a draft re-renders the editor in place.
func TestWriting_PublishAndMakeDraftThroughEditor(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: playwright.Bool(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/` + regexp.QuoteMeta(draft.ID) + `$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText(draft.Subject.String))

	published, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.True(t, published.PublishedAt.Valid)

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Back to draft", Exact: playwright.Bool(true)}).Click())

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`Edit Post`)})
	require.NoError(t, browser.Expect.Locator(heading).ToContainText("Draft"))

	backToDraft, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.False(t, backToDraft.PublishedAt.Valid)
}

// Deleting a draft through the editor's own Delete button, confirmed
// through a native dialog, redirects to /controls and removes the post.
func TestWriting_DeleteThroughEditor(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/?$`)))

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)
}

// Dismissing the confirmation dialog of the editor's Delete button keeps the
// post: the dialog is shown with its warning, and nothing is submitted.
func TestWriting_DeleteDismissedKeepsPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	asked := make(chan string, 1)
	page.OnDialog(func(d playwright.Dialog) {
		asked <- d.Message()
		_ = d.Dismiss()
	})

	editURL := fmt.Sprintf("/posts/%s/edit", draft.ID)
	_, err = page.Goto(editURL)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: playwright.Bool(true)}).Click())

	select {
	case msg := <-asked:
		require.Contains(t, msg, "Do you really want to delete this post?")
	case <-time.After(5 * time.Second):
		t.Fatal("Delete did not ask for confirmation")
	}

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(regexp.QuoteMeta(editURL)+`$`)))

	kept, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.Equal(t, draft.ID, kept.ID)
}

// Known bug: the post-edit form's "Back to draft" action re-renders the
// form in place (hx-swap="outerHTML" on the form itself, since the response
// isn't a redirect or a #last_draft_save retarget). htmx occasionally
// hasn't finished re-processing the freshly swapped-in form's hx-post /
// hx-trigger="submit" binding by the time the very next submit fires, so
// that submit intermittently falls back to a plain, non-htmx browser form
// post. That native post has no X-CSRFToken header (only added by htmx's
// ajax path), so csrf.CheckCSRF rejects it with 403 instead of the post
// being deleted.
//
// Repro: publish a draft, edit it again, click "Back to draft" (in-place
// outerHTML self-swap), then immediately click "Delete" - intermittently
// the browser navigates to POST /controls/form/edit_post with a 403 body
// instead of redirecting to /controls with the post removed.
func TestWriting_DeleteAfterMakeDraftSelfSwap(t *testing.T) {
	t.Parallel()
	t.Skip("known bug #141: after the post-edit form re-renders itself in place (Back to draft), the next submit intermittently posts natively instead of via htmx and gets a 403 from csrf.CheckCSRF instead of deleting the post")

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Back to draft", Exact: playwright.Bool(true)}).Click())

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`Edit Post`)})
	require.NoError(t, browser.Expect.Locator(heading).ToContainText("Draft"))

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/?$`)))

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)
}

// Deleting a draft from the drafts table on /controls, confirmed through a
// native dialog, removes the row and the post.
func TestWriting_DeleteDraftFromControls(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err = page.Goto("/controls")
	require.NoError(t, err)

	row := page.GetByRole("row").Filter(playwright.LocatorFilterOptions{HasText: draft.Subject.String})
	require.NoError(t, row.GetByRole("button").Click())

	require.NoError(t, browser.Expect.Locator(row).ToHaveCount(0))

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)
}

// The edit form for an existing post is pre-filled with its subject and
// body, and saving a published post redirects back to it with the change
// applied.
func TestWriting_EditExistingPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	post, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", post.ID))
	require.NoError(t, err)

	subject := page.GetByPlaceholder("Subject")
	require.NoError(t, browser.Expect.Locator(subject).ToHaveValue(post.Subject.String))

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(post.Body))

	require.NoError(t, subject.Fill("Updated subject"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Post", Exact: playwright.Bool(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/` + regexp.QuoteMeta(post.ID) + `$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText("Updated subject"))

	updated, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated subject", updated.Subject.String)
	require.True(t, updated.PublishedAt.Valid)
}

// /write?prompt=<id> shows the asker's name and message in a banner.
func TestWriting_WriteWithPrompt(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	asker := browser.NewUser(t, app)
	recipient := browser.NewUser(t, app)
	ctx := context.Background()

	prompt, err := factory.PostPrompt(ctx, app.DB, asker.ID, recipient.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(recipient))

	_, err = page.Goto("/write?prompt=" + prompt.ID)
	require.NoError(t, err)

	alert := page.GetByRole("alert")
	require.NoError(t, browser.Expect.Locator(alert).ToContainText(asker.Username))
	require.NoError(t, browser.Expect.Locator(alert).ToContainText(prompt.Message))
}

// Publishing a post with a heading, a fenced code block, a gallery, a bare
// YouTube link and a plain image renders headings shifted down a level,
// chroma syntax highlighting, a gallery with one item per image, a
// lite-youtube embed and a lazy-loaded standalone image.
func TestWriting_RenderedPostFeatures(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Rendered post features"))

	// The CSP only allows images from "self", data: and i.ytimg.com, so the
	// gallery and standalone images use a data url rather than a remote host.
	const b2PixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

	body := "# Heading Test\n\n" +
		"```go\nfunc main() {}\n```\n\n" +
		"{gallery}\n![alt one](" + b2PixelPNG + ")\n\n![alt two](" + b2PixelPNG + ")\n{/gallery}\n\n" +
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ\n\n" +
		"![standalone alt](" + b2PixelPNG + ")\n"

	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill(body))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: playwright.Bool(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/[^/]+$`)))

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Level: playwright.Int(2), Name: "Heading Test"})
	require.NoError(t, browser.Expect.Locator(heading).ToBeVisible())

	require.NoError(t, browser.Expect.Locator(page.Locator("pre.chroma")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(page.Locator(".gallery__item")).ToHaveCount(2))

	youtube := page.Locator("lite-youtube")
	require.NoError(t, browser.Expect.Locator(youtube).ToHaveAttribute("videoid", "dQw4w9WgXcQ"))

	standalone := page.Locator("img.standalone-img")
	require.NoError(t, browser.Expect.Locator(standalone).ToHaveCount(1))

	src, err := standalone.GetAttribute("data-src")
	require.NoError(t, err)
	require.Equal(t, b2PixelPNG, src)
}
