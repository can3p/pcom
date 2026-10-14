//go:build browser

// The grammar check and style review in the post editor: applying a
// suggestion and undoing it, remembering a dismissal, following the author's
// edits, the hourly limit, style notes and what the next review is told, and
// the panel surviving the form being replaced. WireMock answers for Claude.
package browser_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

var _ = func() struct{} {
	wiremock.Register("testdata/wiremock/proofread")

	return struct{}{}
}()

// proofreadEditor opens the edit page of a new draft with this text.
func proofreadEditor(t *testing.T, app *e2e.App, user *model.User, subject, body string) (playwright.Page, *model.Post) {
	t.Helper()

	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.WithSubject(subject), factory.WithBody(body))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))
	_, err = page.Goto("/posts/" + post.ID + "/edit")
	require.NoError(t, err)

	return page, post
}

func button(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
}

// savedBody waits for the autosave to store this body.
func savedBody(t *testing.T, app *e2e.App, id, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		p, err := factory.GetPost(context.Background(), app.DB, id)

		return err == nil && p.Body == want
	}, 10*time.Second, 200*time.Millisecond)
}

// The suggestion follows the text that moves before it; Apply changes the text
// and autosaves it, Undo puts it back.
func TestProofread_ApplyAndUndo(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, post := proofreadEditor(t, app, user, "Cats", "wm-br-apply: the cta sat on the mat.")
	body := page.GetByPlaceholder("Your post goes there")

	require.NoError(t, button(page, "Check grammar").Click())
	card := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "cta → cat"})
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())

	// text typed above the suggestion does not misplace it
	require.NoError(t, body.Click())
	require.NoError(t, page.Keyboard().Press("Control+Home"))
	require.NoError(t, page.Keyboard().Type("So, "))
	// the typing's own autosave lands first, so each save below proves its own trigger
	savedBody(t, app, post.ID, "So, wm-br-apply: the cta sat on the mat.")

	require.NoError(t, button(page, "Apply").Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("So, wm-br-apply: the cat sat on the mat."))
	savedBody(t, app, post.ID, "So, wm-br-apply: the cat sat on the mat.")

	require.NoError(t, button(page, "Undo").Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("So, wm-br-apply: the cta sat on the mat."))
	savedBody(t, app, post.ID, "So, wm-br-apply: the cta sat on the mat.")
}

// A dismissed suggestion stays hidden when the text is checked again, until
// "show again".
func TestProofread_DismissIsRemembered(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-dismiss: a dgo b.")
	card := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "dgo → dog"})

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())
	require.NoError(t, button(page, "Dismiss").Click())
	require.NoError(t, browser.Expect.Locator(card).ToBeHidden())

	// a reload drops the in-memory state: only localStorage remembers
	_, err := page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByText("Grammar: ")).ToBeVisible())

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("All suggestions from this check were dismissed.")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(card).ToBeHidden())

	require.NoError(t, button(page, "1 dismissed · show again").Click())
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())
}

// Changing the text under a suggestion leaves it with nothing to do but go.
func TestProofread_EditedTextIsStale(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-stale: one cta two.")
	body := page.GetByPlaceholder("Your post goes there")

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeVisible())

	_, err := body.Evaluate(`el => { el.focus(); el.setSelectionRange(el.value.indexOf("cta"), el.value.indexOf("cta") + 3) }`, nil)
	require.NoError(t, err)
	require.NoError(t, page.Keyboard().Type("dog"))

	require.NoError(t, browser.Expect.Locator(page.GetByText("You changed this text after the grammar check")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(button(page, "Dismiss")).ToBeVisible())
}

// Once the hour's checks are used the button is disabled; a page that did not
// know it yet is told in words.
func TestProofread_LimitReached(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock(), e2e.WithEnv("PROOFREAD_GRAMMAR_PER_HOUR", "1"))
	user := browser.NewUser(t, app)
	first, post := proofreadEditor(t, app, user, "Cats", "wm-br-limit: a cta here.")

	// the refusal is a 400 on purpose
	second := browser.Page(t, app, browser.As(user), browser.Allow("status of 400"))
	_, err := second.Goto("/posts/" + post.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(second.GetByText("Grammar: 1 of 1 checks left this hour")).ToBeVisible())

	require.NoError(t, button(first, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(first.GetByText("Grammar: 0 of 1 checks left this hour")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(button(first, "Check grammar")).ToBeDisabled())

	require.NoError(t, button(second, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(second.GetByText("You have used all 1 grammar checks for this hour. The next one is available in")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(button(second, "Check grammar")).ToBeDisabled())
}

// A style review: notes are settled one by one, kept notes carry the author's
// reason into the next review, and Start fresh forgets the reviews.
func TestProofread_StyleReview(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-style. Then it ended. The clock was slow.")
	page.OnDialog(func(d playwright.Dialog) { _ = d.Accept() })

	require.NoError(t, button(page, "Style review").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("A solid first draft.")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Review 1 of this draft")).ToBeVisible())

	ending := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "The ending is abrupt."})
	clock := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "The slow clock is never explained."})

	require.NoError(t, ending.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "The ending is abrupt."}).Click())
	require.NoError(t, button(page, "Mark done").Click())
	require.NoError(t, browser.Expect.Locator(ending).ToBeHidden())

	require.NoError(t, clock.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "The slow clock is never explained."}).Click())
	require.NoError(t, button(page, "Keep as is").Click())
	require.NoError(t, page.GetByLabel("Why keep it?", playwright.PageGetByLabelOptions{Exact: new(false)}).Fill("The clock is explained in the sequel"))
	require.NoError(t, button(page, "Keep").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("1 done, 1 kept as is — the next review knows about these.")).ToBeVisible())

	// the answers were saved: a fresh page shows the same
	_, err := page.Reload()
	require.NoError(t, err)
	require.NoError(t, button(page, "Style · 0").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("1 done, 1 kept as is — the next review knows about these.")).ToBeVisible())

	// the second stub answers only a request that carries the reason
	require.NoError(t, button(page, "Review again").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Better. Only the title is left.")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Review 2 of this draft")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Since review 1")).ToBeVisible())

	require.NoError(t, button(page, "Earlier reviews").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("A solid first draft.")).ToBeVisible())

	require.NoError(t, button(page, "Start fresh").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Better. Only the title is left.")).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Review 2 of this draft")).ToBeHidden())

	_, err = page.Reload()
	require.NoError(t, err)
	// the review is loaded when the "left this hour" line appears
	require.NoError(t, browser.Expect.Locator(page.GetByText("Style: 1 of 3 reviews left this hour")).ToBeVisible())
	require.NoError(t, button(page, "Style · 0").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Notes on style, logic and pacing appear here.")).ToBeVisible())
}

// A validation error replaces the form; the suggestions stay in the panel
// and still apply.
func TestProofread_SurvivesFormReplacement(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-replace: a cta here.")
	body := page.GetByPlaceholder("Your post goes there")

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeVisible())

	// a subject over the limit comes back as an error with the form re-rendered
	subject := page.GetByPlaceholder("Subject")
	require.NoError(t, subject.Fill("Cats "+strings.Repeat("x", 120)))
	require.NoError(t, subject.Blur())
	require.NoError(t, browser.Expect.Locator(page.Locator(".field-error")).ToBeVisible())

	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeVisible())
	require.NoError(t, button(page, "Apply").Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("wm-br-replace: a cat here."))
}

// Without a key there are no buttons and no disclosure; with one, a draft
// that has no id yet cannot be reviewed.
func TestProofread_OffAndNewDraft(t *testing.T) {
	t.Parallel()

	off := e2e.Start(t, e2e.WithRealAssets())
	page := browser.Page(t, off, browser.As(browser.NewUser(t, off)))
	_, err := page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Your post goes there")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Check grammar")).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Style review")).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(page.GetByText("an external AI model from Anthropic")).ToBeHidden())

	on := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	page = browser.Page(t, on, browser.As(browser.NewUser(t, on)))
	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(button(page, "Check grammar")).ToBeEnabled())
	require.NoError(t, browser.Expect.Locator(button(page, "Style review")).ToBeDisabled())
	require.NoError(t, browser.Expect.Locator(button(page, "Style review")).ToHaveAttribute("title", "Available once the draft is saved"))

	// the first autosave gives the draft an id
	require.NoError(t, page.GetByPlaceholder("Your post goes there").PressSequentially("Some words"))
	require.NoError(t, browser.Expect.Locator(button(page, "Style review")).ToBeEnabled())
	require.NoError(t, browser.Expect.Locator(page.GetByText("only when you press Check grammar or Style review")).ToBeVisible())
}

// Clicking a card tints its passage in the text; clicking in the text inside
// a suggestion selects its card.
func TestProofread_HighlightAndCaret(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-caret: aaa cta bbb dgo ccc")
	body := page.GetByPlaceholder("Your post goes there")
	cta := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "cta → cat"})
	dgo := page.Locator(".pr-card").Filter(playwright.LocatorFilterOptions{HasText: "dgo → dog"})
	apply := playwright.LocatorGetByRoleOptions{Name: "Apply"}

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(cta.GetByRole("button", apply)).ToBeVisible())

	require.NoError(t, dgo.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "dgo"}).Click())
	require.NoError(t, browser.Expect.Locator(dgo.GetByRole("button", apply)).ToBeVisible())

	box := page.Locator(".pr-hl-box")
	require.NoError(t, browser.Expect.Locator(box).ToHaveCount(1))

	// the tint sits over the passage: right of the text start, inside the box of the textarea
	hl, err := box.BoundingBox()
	require.NoError(t, err)
	ta, err := body.BoundingBox()
	require.NoError(t, err)
	require.Greater(t, hl.X, ta.X+100)
	require.Greater(t, hl.Width, 10.0)
	require.Less(t, hl.X+hl.Width, ta.X+ta.Width)
	require.GreaterOrEqual(t, hl.Y, ta.Y)

	// a caret inside the first passage selects its card
	_, err = body.Evaluate(`el => { el.focus(); const i = el.value.indexOf("cta") + 1; el.setSelectionRange(i, i); el.click() }`, nil)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(cta.GetByRole("button", apply)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(dgo.GetByRole("button", apply)).ToBeHidden())
}

// On a phone the panel is a bottom sheet that collapses to a bar, so the
// buttons under the text stay reachable.
func TestProofread_PhoneBottomSheet(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.WithSubject("Cats"), factory.WithBody("wm-br-apply: the cta sat on the mat."))
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Viewport = &playwright.Size{Width: 390, Height: 844}
	}))
	_, err = page.Goto("/posts/" + post.ID + "/edit")
	require.NoError(t, err)

	require.NoError(t, button(page, "Check grammar").Click())
	bar := button(page, "1 grammar suggestion")
	require.NoError(t, browser.Expect.Locator(bar).ToHaveAttribute("aria-expanded", "true"))
	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeVisible())

	// the sheet sits at the bottom edge of the screen
	sheet, err := page.Locator(".pr-panel").BoundingBox()
	require.NoError(t, err)
	require.InDelta(t, 844.0, sheet.Y+sheet.Height, 2)

	require.NoError(t, bar.Click())
	require.NoError(t, browser.Expect.Locator(bar).ToHaveAttribute("aria-expanded", "false"))
	require.NoError(t, browser.Expect.Locator(button(page, "Apply")).ToBeHidden())

	// with the sheet collapsed the form's buttons can be reached and used
	require.NoError(t, browser.Expect.Locator(button(page, "Publish")).ToBeVisible())
	require.NoError(t, button(page, "Save as Draft").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Draft saved")).ToBeVisible())
}
