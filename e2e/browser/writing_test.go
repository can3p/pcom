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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
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

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Bold"}).Click())

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
// The editor takes the whole content column on a desktop screen, so long posts
// are not written in a strip; its text keeps a margin from the box edge.
func TestWriting_EditorUsesTheColumn(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Viewport = &playwright.Size{Width: 1280, Height: 800}
	}))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	body := page.GetByPlaceholder("Your post goes there")
	box, err := body.BoundingBox()
	require.NoError(t, err)
	// the column is 776px wide less its 16px gutters; an editor that shrinks to
	// its content (the toolbar) is about 420px
	require.GreaterOrEqual(t, box.Width, 700.0)

	padding, err := body.Evaluate(`el => getComputedStyle(el).paddingLeft`, nil)
	require.NoError(t, err)
	px, err := strconv.ParseFloat(strings.TrimSuffix(padding.(string), "px"), 64)
	require.NoError(t, err)
	require.Greater(t, px, 8.0, "the post text must not touch the box edge")
}

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

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/` + regexp.QuoteMeta(draft.ID) + `$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText(draft.Subject.String))

	published, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.True(t, published.PublishedAt.Valid)

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Back to draft", Exact: new(true)}).Click())

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`Edit Post`)})
	require.NoError(t, browser.Expect.Locator(heading).ToContainText("Draft"))

	backToDraft, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.False(t, backToDraft.PublishedAt.Valid)
}

// Pressing Enter in the subject of a published post saves it: the form's
// default button is Save, not the confirmed Delete or Back to draft before it.
func TestWriting_EnterInSubjectSavesPublishedPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	post, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	asked := make(chan string, 1)
	page.OnDialog(func(d playwright.Dialog) {
		asked <- d.Message()
		_ = d.Dismiss()
	})

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", post.ID))
	require.NoError(t, err)

	subject := page.GetByPlaceholder("Subject")
	require.NoError(t, subject.Fill("Renamed with Enter"))
	require.NoError(t, subject.Press("Enter"))

	// saving a published post opens it
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+post.ID+`/?$`)))
	require.Empty(t, asked, "Enter must not ask to delete or unpublish")

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "Renamed with Enter", posts[0].Subject.String)
	require.True(t, posts[0].PublishedAt.Valid, "the post stays published")
}

// "Save as Draft" reports its result next to the buttons, so the click is
// visibly answered even when an autosave already updated the status (#208).
func TestWriting_SaveAsDraftShowsSaved(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	saved := page.GetByRole("status").Filter(playwright.LocatorFilterOptions{HasText: "Draft saved"})
	require.NoError(t, browser.Expect.Locator(saved).ToHaveCount(0))

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Saved by hand"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save as Draft", Exact: new(true)}).Click())

	require.NoError(t, browser.Expect.Locator(saved).ToBeVisible())

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "Saved by hand", posts[0].Subject.String)
	require.False(t, posts[0].PublishedAt.Valid, "Save as Draft keeps it a draft")
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

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: new(true)}).Click())

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

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: new(true)}).Click())

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

// "Back to draft" re-renders the edit form in place (hx-swap="outerHTML" on
// the form itself), and htmx binds the new form only after its settle delay.
// A click on Delete in that window must not submit the form natively: a
// native post carries no X-CSRFToken header and gets a bare 403 (#141). The
// swapped-in buttons stay disabled until the form is bound, so that click
// does nothing and the user's next, ordinary click deletes the post.
func TestWriting_DeleteAfterMakeDraftSelfSwap(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	draft, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	// Click Delete right after the swap, before htmx has bound the new form:
	// the window a quick person could hit. The click must do nothing.
	require.NoError(t, page.AddInitScript(playwright.Script{Content: new(`
		document.addEventListener("htmx:afterSwap", () => {
			document.querySelector("button[value=delete]")?.click()
		})
	`)}))

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Back to draft", Exact: new(true)}).Click())

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`Edit Post`)})
	require.NoError(t, browser.Expect.Locator(heading).ToContainText("Draft"))

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete", Exact: new(true)}).Click())

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
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Post", Exact: new(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/` + regexp.QuoteMeta(post.ID) + `$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText("Updated subject"))

	updated, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated subject", updated.Subject.String)
	require.True(t, updated.PublishedAt.Valid)
}

// b2BodyLimit is the post body limit PostForm.Validate enforces.
const b2BodyLimit = 20_000

// b2SaveBody replaces the body of a published post in the editor and saves
// it.
func b2SaveBody(t *testing.T, page playwright.Page, postID, body string) {
	t.Helper()

	_, err := page.Goto(fmt.Sprintf("/posts/%s/edit", postID))
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill(body))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Post", Exact: new(true)}).Click())
}

// A body over the limit is rejected: the editor shows the error under the
// body, stays on the edit page, and the stored post is unchanged.
func TestWriting_BodyOverLimitShowsError(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	post, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	b2SaveBody(t, page, post.ID, strings.Repeat("a", b2BodyLimit+1))

	require.NoError(t, browser.Expect.Locator(page.GetByText("body should be between 0 and 20000 characters")).ToBeVisible())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+regexp.QuoteMeta(post.ID)+`/edit$`)))

	stored, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, post.Body, stored.Body)
}

// A body exactly at the limit is saved.
func TestWriting_BodyAtLimitSaves(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	post, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	body := strings.Repeat("a", b2BodyLimit)
	b2SaveBody(t, page, post.ID, body)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+regexp.QuoteMeta(post.ID)+`$`)))

	stored, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, body, stored.Body)
}

// The limit is in characters, as the error message says, so a body of
// non-ASCII text under the limit is saved even though it takes more bytes.
func TestWriting_BodyLimitCountsCharacters(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()

	post, err := factory.Post(ctx, app.DB, user.ID, factory.Published())
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	// 15,000 Cyrillic letters: 30,000 bytes of UTF-8.
	body := strings.Repeat("я", 15_000)
	b2SaveBody(t, page, post.ID, body)

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+regexp.QuoteMeta(post.ID)+`$`)))

	stored, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, body, stored.Body)
}

// b2Anonymous names the anonymous viewer in b2RequireSees.
const b2Anonymous = "an anonymous visitor"

// b2Viewers is the audience a visibility test checks a post against: a
// direct connection of the author, a connection of that connection, a
// logged-in stranger and an anonymous visitor.
type b2Viewers struct {
	direct, secondDegree, stranger, anonymous playwright.Page
}

func b2NewViewers(t *testing.T, app *e2e.App, author *core.User) b2Viewers {
	t.Helper()

	ctx := context.Background()
	direct := browser.NewUser(t, app)
	secondDegree := browser.NewUser(t, app)
	stranger := browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, author.ID, direct.ID)
	require.NoError(t, err)
	_, _, err = factory.Connect(ctx, app.DB, direct.ID, secondDegree.ID)
	require.NoError(t, err)

	return b2Viewers{
		direct:       browser.Page(t, app, browser.As(direct), browser.Allow(`404`)),
		secondDegree: browser.Page(t, app, browser.As(secondDegree), browser.Allow(`404`)),
		stranger:     browser.Page(t, app, browser.As(stranger), browser.Allow(`404`)),
		anonymous:    browser.Page(t, app, browser.Allow(`404`)),
	}
}

// b2RequireSees asserts whether viewer can open the post. A post the viewer
// may not see answers 404 to a logged-in user, so its existence isn't
// revealed, and sends an anonymous visitor to the login page.
func b2RequireSees(t *testing.T, viewer playwright.Page, postID, subject string, sees bool, who string) {
	t.Helper()

	resp, err := viewer.Goto("/posts/" + postID)
	require.NoError(t, err)

	if !sees {
		if who == b2Anonymous {
			require.NoError(t, browser.Expect.Page(viewer).ToHaveURL(regexp.MustCompile(`/login`)), "%s can open the post", who)
		} else {
			require.Equal(t, 404, resp.Status(), "%s can open the post", who)
		}
		require.NoError(t, browser.Expect.Locator(viewer.GetByText(subject)).ToHaveCount(0), "%s sees the subject", who)
		return
	}

	require.Equal(t, 200, resp.Status(), "%s can't open the post", who)
	require.NoError(t, browser.Expect.Locator(viewer.Locator(".us-post-header")).ToContainText(subject), who)
}

// The visibility chosen in the editor decides who can open the post: a new
// post defaults to "their connections as well", and changing it on a
// published post takes effect at once, in both directions.
func TestWriting_PostVisibility(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app)
	viewers := b2NewViewers(t, app, author)
	ctx := context.Background()

	page := browser.Page(t, app, browser.As(author))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	direct := page.GetByLabel("Show to direct connections only")
	secondDegree := page.GetByLabel("Show to their connections as well")
	public := page.GetByLabel("Public", playwright.PageGetByLabelOptions{Exact: new(true)})

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(secondDegree).ToBeChecked())

	const subject = "Visibility story"
	require.NoError(t, page.GetByPlaceholder("Subject").Fill(subject))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("Who can read this?"))
	require.NoError(t, direct.Check())
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/([^/]+)$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	postID := postURLRe.FindStringSubmatch(page.URL())[1]

	steps := []struct {
		choose   playwright.Locator
		want     core.PostVisibility
		audience [4]bool // direct, second degree, stranger, anonymous
	}{
		{nil, core.PostVisibilityDirectOnly, [4]bool{true, false, false, false}},
		{secondDegree, core.PostVisibilitySecondDegree, [4]bool{true, true, false, false}},
		{public, core.PostVisibilityPublic, [4]bool{true, true, true, true}},
		{direct, core.PostVisibilityDirectOnly, [4]bool{true, false, false, false}},
	}

	for _, step := range steps {
		if step.choose != nil {
			_, err := page.Goto("/posts/" + postID + "/edit")
			require.NoError(t, err)
			require.NoError(t, step.choose.Check())
			require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Post", Exact: new(true)}).Click())
			require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
		}

		stored, err := factory.GetPost(ctx, app.DB, postID)
		require.NoError(t, err)
		require.Equal(t, step.want, stored.VisibilityRadius)

		b2RequireSees(t, viewers.direct, postID, subject, step.audience[0], "a direct connection")
		b2RequireSees(t, viewers.secondDegree, postID, subject, step.audience[1], "a second-degree connection")
		b2RequireSees(t, viewers.stranger, postID, subject, step.audience[2], "a stranger")
		b2RequireSees(t, viewers.anonymous, postID, subject, step.audience[3], b2Anonymous)
	}

	// The editor shows the stored choice.
	_, err = page.Goto("/posts/" + postID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(direct).ToBeChecked())
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
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/[^/]+$`)))

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Level: new(2), Name: "Heading Test"})
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
