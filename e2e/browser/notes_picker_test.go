//go:build browser

// The post editor's Notes panel: it filters with the tag bar, the search and
// by source and stack, inserts a note at the cursor, appends one, adds a
// quote's attribution, and the post saves with the text and records the notes
// as used.
package browser_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/posts"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

func notesPanel(page playwright.Page) playwright.Locator {
	return page.GetByRole("complementary", playwright.PageGetByRoleOptions{Name: "Notes"})
}

func openNotes(t *testing.T, page playwright.Page) {
	t.Helper()

	require.NoError(t, page.GetByRole("toolbar", playwright.PageGetByRoleOptions{Name: "Formatting"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Notes"}).Click())
}

func TestNotesPicker_FiltersInsertsAndAppends(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()
	now := time.Now().UTC()

	src, err := factory.NoteSource(ctx, app.DB, user.ID)
	require.NoError(t, err)

	_, err = app.DB.ExecContext(ctx, `UPDATE note_sources SET title = 'North of Boston', author = 'Robert Frost' WHERE id = $1`, src.ID)
	require.NoError(t, err)

	alpha := mustNote(t, app, user.ID, factory.WithNoteBody("alpha idea"), factory.WithNoteTags("essay"), factory.CapturedAt(now.Add(-72*time.Hour)))
	quote := mustNote(t, app, user.ID, factory.WithNoteBody("> The only way out is through."), factory.WithNoteTags("essay"),
		factory.FromSource(src.ID), factory.CapturedAt(now.Add(-48*time.Hour)))
	mustNote(t, app, user.ID, factory.WithNoteBody("bravo idea"), factory.WithNoteTags("walk"), factory.CapturedAt(now.Add(-24*time.Hour)))

	for i, body := range []string{"stacked one", "stacked two"} {
		mustNote(t, app, user.ID, factory.WithNoteBody(body), factory.CapturedAt(now.Add(-time.Duration(30-i*5)*time.Minute)))
	}

	_, err = app.DB.ExecContext(ctx, `UPDATE notes SET locator = '12' WHERE id = $1`, quote.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(user))

	_, err = page.Goto("/write")
	require.NoError(t, err)

	body := page.GetByPlaceholder("Your post goes there")
	require.NoError(t, page.GetByPlaceholder("Subject").Fill("With notes"))
	require.NoError(t, body.Fill("Start\n\nEnd"))

	// the typing's own autosave lands first, so the saves below prove their trigger
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))
	postID := b2EditURLRe.FindStringSubmatch(page.URL())[1]
	savedBody(t, app, postID, "Start\n\nEnd")

	panel := notesPanel(page)
	picked := panel.GetByTestId("picker-note")
	card := func(text string) playwright.Locator {
		return picked.Filter(playwright.LocatorFilterOptions{HasText: text})
	}
	link := func(name any) playwright.Locator {
		return panel.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: name})
	}

	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(5))

	// the tag bar narrows the panel
	require.NoError(t, link("#essay").Click())
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(card("bravo idea")).ToHaveCount(0))

	// Insert goes in at the cursor, and saves at once
	_, err = body.Evaluate("el => el.setSelectionRange(5, 5)", nil)
	require.NoError(t, err)
	require.NoError(t, card("alpha idea").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Insert"}).Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("Start\n\nalpha idea\n\nEnd"))
	savedBody(t, app, postID, "Start\n\nalpha idea\n\nEnd")

	// Append goes to the end, a quote with its attribution line
	require.NoError(t, card("only way out").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Append"}).Click())
	want := "Start\n\nalpha idea\n\nEnd\n\n> The only way out is through.\n\n— Robert Frost, *North of Boston*, p. 12\n"
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(want))
	savedBody(t, app, postID, strings.TrimSpace(want))

	// the notes count as used
	store := repo.New(app.DB)

	require.Eventually(t, func() bool {
		a, errA := store.NotePostLinkCount(ctx, alpha.ID)
		q, errQ := store.NotePostLinkCount(ctx, quote.ID)

		return errA == nil && errQ == nil && a == 1 && q == 1
	}, 10*time.Second, 200*time.Millisecond)

	// Open shows the note's edit page in another tab
	open := card("alpha idea").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Open"})
	require.NoError(t, browser.Expect.Locator(open).ToHaveAttribute("href", "/controls/notes/"+alpha.ID+"/edit"))
	require.NoError(t, browser.Expect.Locator(open).ToHaveAttribute("target", "_blank"))

	// by source
	require.NoError(t, panel.GetByText("Sources", playwright.LocatorGetByTextOptions{Exact: new(true)}).Click())
	require.NoError(t, link("North of Boston").Click())
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(card("only way out")).ToBeVisible())
	require.NoError(t, link("North of Boston").Click())
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(2))

	// one more tap on the chip excludes the tag; the search narrows what is left
	require.NoError(t, link("#essay").Click())
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(3))

	search := panel.GetByRole("searchbox", playwright.LocatorGetByRoleOptions{Name: "Search notes"})
	require.NoError(t, search.Fill("bravo"))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(card("bravo idea")).ToBeVisible())
	require.NoError(t, search.Fill(""))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(3))

	// by stack: the heading opens the stack alone
	require.NoError(t, link(regexp.MustCompile(`, 2 notes$`)).Click())
	require.NoError(t, browser.Expect.Locator(picked).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(card("stacked one")).ToBeVisible())

	// the panel closes
	require.NoError(t, panel.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close notes"}).Click())
	require.NoError(t, browser.Expect.Locator(panel).ToBeHidden())

	// the note's own form has no Notes button: it is the post editor's
	_, err = page.Goto("/controls/notes/" + alpha.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("toolbar", playwright.PageGetByRoleOptions{Name: "Formatting"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Notes", Exact: new(true)})).ToHaveCount(0))
}

// A post that has no id yet takes a note: the first save gives it one, and
// only then is the note recorded as used in it.
func TestNotesPicker_InsertBeforeTheFirstSave(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	note := mustNote(t, app, user.ID, factory.WithNoteBody("first thought"))
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	body := page.GetByPlaceholder("Your post goes there")
	openNotes(t, page)
	require.NoError(t, notesPanel(page).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Insert"}).Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("first thought\n"))

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))
	postID := b2EditURLRe.FindStringSubmatch(page.URL())[1]
	savedBody(t, app, postID, "first thought")

	store := repo.New(app.DB)

	require.Eventually(t, func() bool {
		n, err := store.NotePostLinkCount(context.Background(), note.ID)

		return err == nil && n == 1
	}, 10*time.Second, 200*time.Millisecond)
}

// Opening the Notes panel closes the proofread panel, and a grammar check
// closes the Notes panel.
func TestNotesPicker_TakesTurnsWithProofread(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteBody("a note"))
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-apply: the cta sat on the mat.")
	suggestions := page.GetByRole("complementary", playwright.PageGetByRoleOptions{Name: "Suggestions"})

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeVisible())

	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(notesPanel(page)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeHidden())

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(notesPanel(page)).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeVisible())
}

// A save that fails validation draws the editor again; it keeps the Notes
// button.
func TestNotesPicker_ButtonSurvivesAFailedSave(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user), browser.Allow(`400|422`))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Subject").Fill(strings.Repeat("long ", posts.DefaultPostSubjectMaxLength)))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("text"))
	require.NoError(t, browser.Expect.Locator(page.Locator(".field-error")).ToBeVisible())

	require.NoError(t, browser.Expect.Locator(page.GetByRole("toolbar", playwright.PageGetByRoleOptions{Name: "Formatting"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Notes"})).ToBeVisible())
}
