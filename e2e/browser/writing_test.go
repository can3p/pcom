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
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/mxschmitt/playwright-go"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// b2EditURLRe pulls the post id out of the "/posts/<id>/edit" url that
// HX-Replace-Url lands the browser on when a new post is saved as a draft.
var b2EditURLRe = regexp.MustCompile(`/posts/([^/]+)/edit$`)

// padOf is the author's plain scratch pad: its revision and body, revision 0
// when they never wrote one.
func padOf(t *testing.T, app *e2e.App, userID string) (int, string) {
	t.Helper()

	return promptPadOf(t, app, userID, "")
}

// promptPadOf is padOf for the pad of a prompt, the plain pad for "".
func promptPadOf(t *testing.T, app *e2e.App, userID, promptID string) (int, string) {
	t.Helper()

	var (
		revision int
		body     string
	)

	err := app.DB.QueryRowContext(context.Background(),
		`SELECT revision, body FROM post_scratches WHERE user_id = $1 AND prompt_id IS NOT DISTINCT FROM NULLIF($2, '')::uuid`, userID, promptID).Scan(&revision, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ""
	}

	require.NoError(t, err)

	return revision, body
}

// savedPad waits for an autosave to store this body in the author's plain pad.
func savedPad(t *testing.T, app *e2e.App, userID, want string) {
	t.Helper()
	savedPromptPad(t, app, userID, "", want)
}

// savedPromptPad is savedPad for the pad of a prompt.
func savedPromptPad(t *testing.T, app *e2e.App, userID, promptID, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, body := promptPadOf(t, app, userID, promptID)

		return strings.TrimSpace(body) == strings.TrimSpace(want)
	}, 10*time.Second, 100*time.Millisecond)
}

// typeAtEnd types text after what the body holds, with real key presses, so
// that the form's autosave sees them.
func typeAtEnd(t *testing.T, page playwright.Page, text string) {
	t.Helper()

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Click())
	require.NoError(t, page.Keyboard().Press("ControlOrMeta+End"))
	require.NoError(t, body.PressSequentially(text))
}

// popups counts the windows the page opens.
func popups(page playwright.Page) *atomic.Int32 {
	var n atomic.Int32

	page.OnPopup(func(playwright.Page) { n.Add(1) })

	return &n
}

// requireNoPopup fails if a window opens within a second.
func requireNoPopup(t *testing.T, n *atomic.Int32) {
	t.Helper()

	require.Never(t, func() bool { return n.Load() > 0 }, time.Second, 50*time.Millisecond, "a preview window was opened")
}

func writeButton(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
}

// A new post is kept as it is typed, in the author's scratch pad, and is no
// draft: typing creates no post, a reload brings the text back, and Save as
// Draft creates exactly one post, empties the pad and moves to its edit page.
func TestWriting_NewPostIsKeptUntilSavedAsDraft(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	ctx := context.Background()

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("My new post"))
	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Click())
	require.NoError(t, body.PressSequentially("Hello world"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Bold"}).Click())

	status := page.Locator("#last_draft_save")
	require.NoError(t, browser.Expect.Locator(status).ToContainText("Not saved as a draft yet"))
	savedPad(t, app, user.ID, "Hello world**bold**")
	// a reload shows the pad, not what the browser remembers of the fields
	require.NoError(t, browser.Expect.Locator(page.Locator("form.ed-form")).ToHaveAttribute("autocomplete", "off"))

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)

	// the text comes back when the page is opened again
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Subject")).ToHaveValue("My new post"))
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("Hello world**bold**"))
	require.NoError(t, browser.Expect.Locator(status.Locator(".status")).ToHaveText(regexp.MustCompile(`^Not saved as a draft yet · kept .+`)))
	require.NoError(t, browser.Expect.Locator(status.Locator(".status span[title]")).ToHaveCount(1), "the time")

	require.NoError(t, writeButton(page, "Save as Draft").Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))
	require.NoError(t, browser.Expect.Locator(status).ToContainText("Last Updated"))

	posts, err = factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "My new post", lo.FromPtr(posts[0].Subject))
	require.Contains(t, posts[0].Body, "**bold**")
	require.Nil(t, posts[0].PublishedAt)

	// the pad is empty, and what is left to remove is the draft
	_, padBody := padOf(t, app, user.ID)
	require.Empty(t, padBody)
	require.NoError(t, browser.Expect.Locator(writeButton(page, "Delete")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(writeButton(page, "Discard")).ToHaveCount(0))

	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(""))
}

// Discard asks first, then empties the pad: the text is gone and no post was
// made. Dismissing the question keeps it.
func TestWriting_DiscardEmptiesTheNewPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	accept := false
	asked := ""

	page.OnDialog(func(d playwright.Dialog) {
		asked = d.Message()

		if accept {
			_ = d.Accept()
		} else {
			_ = d.Dismiss()
		}
	})

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Not wanted"))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").PressSequentially("some text"))
	savedPad(t, app, user.ID, "some text")

	require.NoError(t, writeButton(page, "Discard").Click())
	require.Eventually(t, func() bool { return asked != "" }, 5*time.Second, 50*time.Millisecond)
	require.Contains(t, asked, "It is not a draft and cannot be brought back")
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/write$`)))
	require.Never(t, func() bool {
		_, body := padOf(t, app, user.ID)

		return body == ""
	}, time.Second, 50*time.Millisecond, "a dismissed Discard emptied the pad")

	accept = true

	require.NoError(t, writeButton(page, "Discard").Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/?$`)))

	_, padBody := padOf(t, app, user.ID)
	require.Empty(t, padBody)

	posts, err := factory.ListPosts(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Empty(t, posts)

	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Your post goes there")).ToHaveValue(""))
}

// The preview link of a new post opens its scratch pad as the post page draws
// a draft, before there is a draft; with nothing typed it opens nothing.
func TestWriting_PreviewOpensTheNewPost(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	link := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Open post preview"})
	opened := popups(page)
	require.NoError(t, link.Click())
	requireNoPopup(t, opened) // nothing was typed, nothing to preview

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Preview me"))
	// no pause for the autosave: the link saves first
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("Some **bold** words"))

	preview, err := page.ExpectPopup(func() error { return link.Click() })
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(preview.Locator(".us-post-header")).ToHaveText("Preview me"))
	require.NoError(t, browser.Expect.Locator(preview.Locator(".us-post-body strong")).ToHaveText("bold"))
	require.NoError(t, browser.Expect.Locator(preview.Locator(".post-foot")).ToHaveCount(0), "no comments on an unsaved text")

	// once saved as a draft, the link opens the draft
	require.NoError(t, writeButton(page, "Save as Draft").Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))
	require.NoError(t, browser.Expect.Locator(link).ToHaveAttribute("href", regexp.MustCompile(`/posts/[^/]+\?edit_preview=true$`)))

	draftPreview, err := page.ExpectPopup(func() error { return link.Click() })
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(draftPreview.Locator(".us-post-header")).ToHaveText("Preview me"))
	require.Contains(t, draftPreview.URL(), "/posts/")
}

// The notice about the scratch pad shows on a new post until the author says
// Got it; it is stored for the account, so another browser doesn't show it
// again, and a draft's editor never has it.
func TestWriting_ScratchNoticeShownUntilDismissed(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))
	notice := page.Locator("#scratch-notice")

	_, err := page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(notice).ToContainText("becomes a draft only when you press Save as Draft"))

	require.NoError(t, notice.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Got it"}).Click())
	require.NoError(t, browser.Expect.Locator(notice).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Subject")).ToBeVisible(), "the editor stays")

	// a draft's editor never has it, even for an account that has not dismissed it
	fresh := browser.NewUser(t, app)
	draft := lo.Must(factory.Post(context.Background(), app.DB, fresh.ID))
	freshPage := browser.Page(t, app, browser.As(fresh))
	_, err = freshPage.Goto("/posts/" + draft.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(freshPage.GetByPlaceholder("Subject")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(freshPage.Locator("#scratch-notice")).ToHaveCount(0))

	// another browser context of the same user
	other := browser.Page(t, app, browser.As(user))
	_, err = other.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(other.GetByPlaceholder("Subject")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(other.Locator("#scratch-notice")).ToHaveCount(0))

	// a different account still sees it
	stranger := browser.Page(t, app, browser.As(browser.NewUser(t, app)))
	_, err = stranger.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(stranger.Locator("#scratch-notice")).ToBeVisible())
}

// Two tabs on one pad: the one that saves second is told, stops saving, and
// has three ways out that each lose nothing the author didn't choose to
// lose: load the other text, keep its own over it, or save its own as a
// draft. A pad emptied by a draft says so.
func TestWriting_TwoTabsOnOnePad(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	tabA := browser.Page(t, app, browser.As(user))
	tabB, err := tabA.Context().NewPage()
	require.NoError(t, err)

	for _, tab := range []playwright.Page{tabA, tabB} {
		_, err = tab.Goto("/write")
		require.NoError(t, err)
	}

	noticeOf := func(tab playwright.Page) playwright.Locator {
		return tab.GetByRole("alert").Filter(playwright.LocatorFilterOptions{HasText: "The text here is no longer being saved"})
	}
	bodyOf := func(tab playwright.Page) playwright.Locator { return tab.GetByPlaceholder("Your post goes there") }

	// A saves first; B's first save finds the pad taken
	typeAtEnd(t, tabA, "from A")
	savedPad(t, app, user.ID, "from A")
	typeAtEnd(t, tabB, "from B")
	require.NoError(t, browser.Expect.Locator(noticeOf(tabB)).ToContainText("This new post was changed on another device or tab at "))
	_, padBody := padOf(t, app, user.ID)
	require.Equal(t, "from A", padBody, "the losing tab writes nothing")

	// a preview of the other tab's text is not opened
	opened := popups(tabB)
	require.NoError(t, tabB.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Open post preview"}).Click())
	requireNoPopup(t, opened)
	require.NoError(t, browser.Expect.Locator(noticeOf(tabB)).ToBeVisible())

	// Load the newer text: the page is the pad's text
	require.NoError(t, writeButton(tabB, "Load the newer text").Click())
	require.NoError(t, browser.Expect.Locator(bodyOf(tabB)).ToHaveValue("from A"))
	require.NoError(t, browser.Expect.Locator(noticeOf(tabB)).ToHaveCount(0))

	// B saves on top of it; now A is behind
	typeAtEnd(t, tabB, " and B")
	savedPad(t, app, user.ID, "from A and B")
	typeAtEnd(t, tabA, " and A")
	require.NoError(t, browser.Expect.Locator(noticeOf(tabA)).ToBeVisible())

	// Keep mine: A's text replaces the pad, and A saves again
	require.NoError(t, writeButton(tabA, "Keep mine").Click())
	require.NoError(t, browser.Expect.Locator(noticeOf(tabA)).ToHaveCount(0))
	savedPad(t, app, user.ID, "from A and A")
	typeAtEnd(t, tabA, "!")
	savedPad(t, app, user.ID, "from A and A!")
	require.NoError(t, browser.Expect.Locator(noticeOf(tabA)).ToHaveCount(0))

	// ... and B meets the notice in turn; Save mine as a draft leaves the pad alone
	typeAtEnd(t, tabB, "?")
	require.NoError(t, browser.Expect.Locator(noticeOf(tabB)).ToBeVisible())
	require.NoError(t, writeButton(tabB, "Save mine as a draft").Click())
	require.NoError(t, browser.Expect.Page(tabB).ToHaveURL(b2EditURLRe))

	posts, err := factory.ListPosts(context.Background(), app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "from A and B?", posts[0].Body)
	_, padBody = padOf(t, app, user.ID)
	require.Equal(t, "from A and A!", padBody)

	// A saves its text as a draft, which empties the pad; a third tab that
	// still shows the old text is told where it went
	tabC, err := tabA.Context().NewPage()
	require.NoError(t, err)
	_, err = tabC.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, writeButton(tabA, "Save as Draft").Click())
	require.NoError(t, browser.Expect.Page(tabA).ToHaveURL(b2EditURLRe))

	typeAtEnd(t, tabC, " more")
	require.NoError(t, browser.Expect.Locator(noticeOf(tabC)).ToContainText("This text was saved as a draft in another tab"))
	require.NoError(t, browser.Expect.Locator(noticeOf(tabC).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "saved as a draft"})).ToHaveAttribute("href", b2EditURLRe))
	require.NoError(t, browser.Expect.Locator(tabC.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Load the newer text"})).ToHaveCount(0))
}

// One tab's own saves never conflict with each other: a save asked for while
// the previous one is on its way is sent after its response, with the
// revision that response left in the form. The first response is held back to
// make the overlap certain.
func TestWriting_BurstOfTypingNeverConflicts(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	var (
		first   atomic.Bool
		fetched = make(chan struct{})
		release = make(chan struct{})
	)

	require.NoError(t, page.Route("**/controls/form/edit_post", func(route playwright.Route) {
		if !first.CompareAndSwap(false, true) {
			_ = route.Continue()

			return
		}

		// answer from another goroutine: the handler must not block the driver
		go func() {
			resp, err := route.Fetch()
			if err != nil {
				return
			}

			close(fetched)
			<-release
			_ = route.Fulfill(playwright.RouteFulfillOptions{Response: resp})
		}()
	}))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	form := page.Locator("form.ed-form")
	save := func(body string) {
		_, err := form.Evaluate(`(form, body) => {
			form.elements.body.value = body
			form.dispatchEvent(new CustomEvent("postform:save"))
		}`, body)
		require.NoError(t, err)
	}

	save("one")
	<-fetched // the server has stored "one", the form doesn't know yet
	save("two")
	close(release)

	savedPad(t, app, user.ID, "two")

	revision, _ := padOf(t, app, user.ID)
	require.Equal(t, 2, revision, "two saves, in order")
	require.NoError(t, browser.Expect.Locator(page.GetByRole("alert")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.Locator("#last_draft_save")).ToContainText("Not saved as a draft yet"))
}

// A pad discarded in another tab is said to be discarded: no link, nothing to
// load, the two choices that remain.
func TestWriting_DiscardedInAnotherTab(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	tabA := browser.Page(t, app, browser.As(user))
	tabA.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	_, err := tabA.Goto("/write")
	require.NoError(t, err)
	typeAtEnd(t, tabA, "to be discarded")
	savedPad(t, app, user.ID, "to be discarded")

	tabB, err := tabA.Context().NewPage()
	require.NoError(t, err)
	_, err = tabB.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(tabB.GetByPlaceholder("Your post goes there")).ToHaveValue("to be discarded"))

	require.NoError(t, writeButton(tabA, "Discard").Click())
	require.NoError(t, browser.Expect.Page(tabA).ToHaveURL(regexp.MustCompile(`/controls/?$`)))

	typeAtEnd(t, tabB, " and more")

	notice := tabB.GetByRole("alert").Filter(playwright.LocatorFilterOptions{HasText: "The text here is no longer being saved"})
	require.NoError(t, browser.Expect.Locator(notice).ToContainText("This text was discarded in another tab"))
	require.NoError(t, browser.Expect.Locator(notice.GetByRole("link")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(tabB.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Load the newer text"})).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(tabB.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Keep mine"})).ToBeVisible())
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
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText(lo.FromPtr(draft.Subject)))

	published, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, published.PublishedAt)

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", draft.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Back to draft", Exact: new(true)}).Click())

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`Edit Post`)})
	require.NoError(t, browser.Expect.Locator(heading).ToContainText("Draft"))

	backToDraft, err := factory.GetPost(ctx, app.DB, draft.ID)
	require.NoError(t, err)
	require.Nil(t, backToDraft.PublishedAt)
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
	require.Equal(t, "Renamed with Enter", lo.FromPtr(posts[0].Subject))
	require.NotNil(t, posts[0].PublishedAt, "the post stays published")
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

	// a long body scrolled down: saving must not replace the textarea, which
	// would lose the writer's place
	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Fill(strings.Repeat("A line of a long post.\n", 200)))
	_, err = body.Evaluate(`el => { el.scrollTop = 400; el.keptAcrossSave = true }`, nil)
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save as Draft", Exact: new(true)}).Click())

	require.NoError(t, browser.Expect.Locator(saved).ToBeVisible())
	// it sits on the buttons' line, centred with them
	savedBox, err := saved.BoundingBox()
	require.NoError(t, err)
	buttonBox, err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save as Draft", Exact: new(true)}).BoundingBox()
	require.NoError(t, err)
	require.InDelta(t, buttonBox.Y+buttonBox.Height/2, savedBox.Y+savedBox.Height/2, 2, "the status is not centred with the buttons")

	kept, err := body.Evaluate(`el => [el.keptAcrossSave === true, el.scrollTop]`, nil)
	require.NoError(t, err)
	require.Equal(t, []any{true, 400}, kept, "the textarea was replaced or scrolled back")

	// like every form result, it goes away on its own
	require.NoError(t, browser.Expect.Locator(saved).ToHaveCount(0, playwright.LocatorAssertionsToHaveCountOptions{
		Timeout: playwright.Float(6000),
	}))

	posts, err := factory.ListPosts(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "Saved by hand", lo.FromPtr(posts[0].Subject))
	require.Nil(t, posts[0].PublishedAt, "Save as Draft keeps it a draft")
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

	row := page.GetByRole("row").Filter(playwright.LocatorFilterOptions{HasText: lo.FromPtr(draft.Subject)})
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
	require.NoError(t, browser.Expect.Locator(subject).ToHaveValue(lo.FromPtr(post.Subject)))

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(post.Body))

	require.NoError(t, subject.Fill("Updated subject"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Post", Exact: new(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/` + regexp.QuoteMeta(post.ID) + `$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	require.NoError(t, browser.Expect.Locator(page.Locator(".us-post-header")).ToContainText("Updated subject"))

	updated, err := factory.GetPost(ctx, app.DB, post.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated subject", lo.FromPtr(updated.Subject))
	require.NotNil(t, updated.PublishedAt)
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

func b2NewViewers(t *testing.T, app *e2e.App, author *model.User) b2Viewers {
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
		want     model.PostVisibility
		audience [4]bool // direct, second degree, stranger, anonymous
	}{
		{nil, model.PostVisibilityDirectOnly, [4]bool{true, false, false, false}},
		{secondDegree, model.PostVisibilitySecondDegree, [4]bool{true, true, false, false}},
		{public, model.PostVisibilityPublic, [4]bool{true, true, true, true}},
		{direct, model.PostVisibilityDirectOnly, [4]bool{true, false, false, false}},
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

// An author with email subscribers is offered to mail a post to them only
// while Public is selected: checked on a first publish, unchecked when a
// published post turns public, and gone once the post was mailed.
func TestWriting_EmailSubscribersOffer(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app, factory.Subscribable())
	ctx := context.Background()

	reader := "writing-reader-" + author.ID + "@example.test"
	_, err := factory.EmailSubscription(ctx, app.DB, author.ID, reader)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(author))
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	direct := page.GetByLabel("Show to direct connections only")
	public := page.GetByLabel("Public and your 1 email subscriber", playwright.PageGetByLabelOptions{Exact: new(true)})
	offer := page.GetByLabel("Email this post to 1 subscriber")

	_, err = page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(offer).ToBeHidden())
	require.NoError(t, public.Check())
	require.NoError(t, browser.Expect.Locator(offer).ToBeChecked())
	require.NoError(t, direct.Check())
	require.NoError(t, browser.Expect.Locator(offer).ToBeHidden())
	require.NoError(t, public.Check())

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("For my subscribers"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())

	postURLRe := regexp.MustCompile(`/posts/([^/]+)$`)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))
	postID := postURLRe.FindStringSubmatch(page.URL())[1]

	mailed, err := factory.GetPost(ctx, app.DB, postID)
	require.NoError(t, err)
	require.NotNil(t, mailed.SubscribersMailedAt)

	_, err = page.Goto("/posts/" + postID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(public).ToBeChecked())
	require.NoError(t, browser.Expect.Locator(offer).ToHaveCount(0))

	published, err := factory.Post(ctx, app.DB, author.ID, factory.Published())
	require.NoError(t, err)

	_, err = page.Goto("/posts/" + published.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(offer).ToBeHidden())
	require.NoError(t, public.Check())
	require.NoError(t, browser.Expect.Locator(offer).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(offer).Not().ToBeChecked())

	// unchecked on a first publish, kept through a validation error, the post
	// goes to nobody by email
	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, public.Check())
	require.NoError(t, offer.Uncheck())

	subject := page.GetByPlaceholder("Subject")
	publish := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)})
	require.NoError(t, subject.Fill(strings.Repeat("x", 101)))
	require.NoError(t, publish.Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("subject should be between 0 and 100 characters")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(offer).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(offer).Not().ToBeChecked())

	const kept = "Kept from my subscribers"
	require.NoError(t, subject.Fill(kept))
	require.NoError(t, publish.Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(postURLRe))

	notMailed, err := factory.GetPost(ctx, app.DB, postURLRe.FindStringSubmatch(page.URL())[1])
	require.NoError(t, err)
	require.Nil(t, notMailed.SubscribersMailedAt)
	app.NoMails(t, reader, func(m tommy.Mail) bool { return strings.Contains(m.Subject, kept) })
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

	// the answer has a scratch pad of its own: kept across a reload, and not
	// the text of an ordinary new post
	require.NoError(t, browser.Expect.Locator(page.Locator("#show_preview")).ToHaveAttribute("href", "/write/preview?prompt="+prompt.ID))
	typeAtEnd(t, page, "my answer")
	savedPromptPad(t, app, recipient.ID, prompt.ID, "my answer")

	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Your post goes there")).ToHaveValue("my answer"))

	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Your post goes there")).ToHaveValue(""))
	_, plain := padOf(t, app, recipient.ID)
	require.Empty(t, plain, "the answer was not written to the plain pad")
}

// Publishing a post with a heading, a fenced code block, a gallery, a bare
// YouTube link and a plain image renders headings shifted down a level,
// chroma syntax highlighting, a gallery with one item per image, a
// lite-youtube embed and a lazy-loaded standalone image. The lines right
// under an image are its caption; in the gallery everything up to the next
// image is.
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
		"{gallery}\n![alt one](" + b2PixelPNG + ")\nFirst caption\n\nmore about one\n\n![alt two](" + b2PixelPNG + ")\n{/gallery}\n\n" +
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ\n\n" +
		"![standalone alt](" + b2PixelPNG + ")\nStandalone caption\n"

	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill(body))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/[^/]+$`)))

	heading := page.GetByRole("heading", playwright.PageGetByRoleOptions{Level: new(2), Name: "Heading Test"})
	require.NoError(t, browser.Expect.Locator(heading).ToBeVisible())

	require.NoError(t, browser.Expect.Locator(page.Locator("pre.chroma")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(page.Locator(".gallery__item")).ToHaveCount(2))

	galleryCaption := page.Locator(".gallery__item").First().Locator("figcaption")
	require.NoError(t, browser.Expect.Locator(galleryCaption).ToHaveText("First caption more about one", playwright.LocatorAssertionsToHaveTextOptions{UseInnerText: new(true)}))
	require.NoError(t, browser.Expect.Locator(page.Locator(".gallery__item").Nth(1).Locator("figcaption")).ToHaveCount(0))

	standaloneCaption := page.Locator("figure.post-figure figcaption")
	require.NoError(t, browser.Expect.Locator(standaloneCaption).ToHaveText("Standalone caption"))

	youtube := page.Locator("lite-youtube")
	require.NoError(t, browser.Expect.Locator(youtube).ToHaveAttribute("videoid", "dQw4w9WgXcQ"))

	standalone := page.Locator("img.standalone-img")
	require.NoError(t, browser.Expect.Locator(standalone).ToHaveCount(1))

	src, err := standalone.GetAttribute("data-src")
	require.NoError(t, err)
	require.Equal(t, b2PixelPNG, src)
}

// Uploading an image into a post leaves the caret on the line under it, so
// what the author types next is the image's caption.
func TestWriting_TypingAfterUploadAddsCaption(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, body.Fill("Intro"))

	require.NoError(t, page.Locator("input[type=file]").SetInputFiles(playwright.InputFile{
		Name:     "beach.png",
		MimeType: "image/png",
		Buffer:   b5PNGBytes(),
	}))
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(regexp.MustCompile(`^Intro\n!\[\]\(\S+\)\n$`)))

	require.NoError(t, page.Keyboard().Type("At the beach"))
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(regexp.MustCompile(`^Intro\n!\[\]\(\S+\)\nAt the beach$`)))

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Captioned"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Publish", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/[^/]+$`)))

	require.NoError(t, browser.Expect.Locator(page.Locator("figure.post-figure figcaption")).ToHaveText("At the beach"))
}

// Wrapping freshly added images into a gallery and opening the preview at
// once shows the gallery: the preview link waits for the edit to be saved
// rather than opening the draft as it was before the edit.
func TestWriting_PreviewRightAfterGalleryShowsGallery(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	const pixel = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	images := "![one](" + pixel + ")\n\n![two](" + pixel + ")"

	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.WithSubject("Gallery preview"), factory.WithBody(images))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto(fmt.Sprintf("/posts/%s/edit", post.ID))
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Your post goes there").SelectText())
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Gallery"}).Click())

	preview, err := page.ExpectPopup(func() error {
		return page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Open post preview"}).Click()
	})
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(preview.Locator(".gallery__item")).ToHaveCount(2))

	saved, err := factory.GetPost(context.Background(), app.DB, post.ID)
	require.NoError(t, err)
	require.Contains(t, saved.Body, "{gallery}")
}

// Inside a quote the editor keeps the `>` markers in place: the Quote button
// quotes and unquotes every selected line, Enter continues the quote and a
// third Enter leaves it, Backspace right after the markers joins the line with
// the previous one (Shift+Backspace deletes one character), and pasted lines
// are quoted. Shift+Enter never continues a quote or a list.
func TestWriting_BlockquoteEditing(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	body := page.GetByPlaceholder("Your post goes there")
	quote := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Quote"})
	press := func(keys ...string) {
		t.Helper()
		for _, key := range keys {
			require.NoError(t, body.Press(key))
		}
	}
	expectBody := func(value string) {
		t.Helper()
		require.NoError(t, browser.Expect.Locator(body).ToHaveValue(value))
	}
	// caret moves the caret to an offset of the body, or selects a range
	caret := func(start, end int) {
		t.Helper()
		_, err := body.Evaluate(`(el, [s, e]) => { el.focus(); el.setSelectionRange(s, e) }`, []int{start, end})
		require.NoError(t, err)
	}

	require.NoError(t, body.Click())
	require.NoError(t, quote.Click())
	require.NoError(t, body.PressSequentially("the quote begins"))
	press("Enter")
	require.NoError(t, body.PressSequentially("and continues"))
	expectBody("> the quote begins\n> and continues")

	// an accidental line break is removed with one Backspace
	caret(len("> the quote begins\n> "), len("> the quote begins\n> "))
	press("Backspace")
	expectBody("> the quote begins and continues")
	// the join is one undo step
	press("ControlOrMeta+z")
	expectBody("> the quote begins\n> and continues")
	press("ControlOrMeta+Shift+z")
	expectBody("> the quote begins and continues")
	caret(len("> the quote begins and continues"), len("> the quote begins and continues"))

	// Shift+Backspace is the plain textarea behavior
	press("End", "Enter", "Shift+Backspace")
	expectBody("> the quote begins and continues\n>")
	press("Space")

	// Enter on an empty quote line makes a paragraph break, the next one leaves the quote
	press("Enter", "Enter")
	require.NoError(t, body.PressSequentially("after"))
	expectBody("> the quote begins and continues\n\nafter")

	// pasting several lines into the quote quotes every line
	caret(len("> the quote begins and continues"), len("> the quote begins and continues"))
	_, err = body.Evaluate(`el => {
		const data = new DataTransfer()
		data.setData("text/plain", "\nfirst\n\nsecond")
		el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }))
	}`, nil)
	require.NoError(t, err)
	expectBody("> the quote begins and continues\n> first\n>\n> second\n\nafter")

	// the Quote button removes the quote from every selected line, and puts it back
	quoted := "> the quote begins and continues\n> first\n>\n> second"
	caret(0, len(quoted))
	require.NoError(t, quote.Click())
	expectBody("the quote begins and continues\nfirst\n\nsecond\n\nafter")
	require.NoError(t, quote.Click())
	expectBody(quoted + "\n\nafter")

	// Shift+Enter is a plain line break in a list too
	all := len(quoted + "\n\nafter")
	caret(all, all)
	press("Enter")
	require.NoError(t, body.PressSequentially("- item"))
	press("Shift+Enter")
	expectBody(quoted + "\n\nafter\n- item\n")
}
