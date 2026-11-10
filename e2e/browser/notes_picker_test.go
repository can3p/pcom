//go:build browser

// The post editor's sidebar and its Notes tab: the tabs and the collapsed
// strip beside the editor box (remembered per browser), the search with the
// tags on show, Insert and Append saving the post, Edit and Delete on a card,
// and the icon buttons' labels.
package browser_test

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
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

func sideTab(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("tab", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
}

func notesPanel(page playwright.Page) playwright.Locator {
	return page.GetByRole("tabpanel", playwright.PageGetByRoleOptions{Name: "Notes", Exact: new(true)})
}

func checksPanel(page playwright.Page) playwright.Locator {
	return page.GetByRole("tabpanel", playwright.PageGetByRoleOptions{Name: "Checks", Exact: new(true)})
}

func sideToggle(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
}

// strip is the collapsed sidebar.
func strip(page playwright.Page) playwright.Locator {
	return page.GetByTestId("sidebar-strip")
}

// openNotes shows the Notes tab: from the strip when the sidebar is
// collapsed (a fresh browser starts so), else from its tab.
func openNotes(t *testing.T, page playwright.Page) {
	t.Helper()

	show := sideToggle(page, "Show Notes")
	if visible, err := show.IsVisible(); err == nil && visible {
		require.NoError(t, show.Click())
	} else {
		require.NoError(t, sideTab(page, "Notes").Click())
	}

	require.NoError(t, browser.Expect.Locator(notesPanel(page)).ToBeVisible())
}

func pickerCards(page playwright.Page) playwright.Locator {
	return notesPanel(page).GetByTestId("picker-note")
}

func pickerCard(page playwright.Page, text string) playwright.Locator {
	return pickerCards(page).Filter(playwright.LocatorFilterOptions{HasText: text})
}

func searchNotes(t *testing.T, page playwright.Page, q string) {
	t.Helper()

	require.NoError(t, notesPanel(page).GetByRole("searchbox", playwright.LocatorGetByRoleOptions{Name: "Search notes"}).Fill(q))
}

// The search takes words and tags like the notes list does; the most used
// tags under it and each card's tags add #tag to it; there are no source
// filters; Insert and Append put the text into the post and save it, and the
// notes count as used.
func TestNotesPicker_SearchInsertsAndAppends(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()
	now := time.Now().UTC()

	src, err := factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("North of Boston"))
	require.NoError(t, err)

	alpha := mustNote(t, app, user.ID, factory.WithNoteBody("alpha idea"), factory.WithNoteTags("essay"), factory.CapturedAt(now.Add(-72*time.Hour)))
	quote := mustNote(t, app, user.ID, factory.WithNoteBody("> The only way out is through."), factory.WithNoteTags("essay"),
		factory.FromSource(src.ID), factory.CapturedAt(now.Add(-48*time.Hour)))
	mustNote(t, app, user.ID, factory.WithNoteBody("bravo idea #later"), factory.WithNoteTags("walk"), factory.WithNoteTextTags("later"),
		factory.CapturedAt(now.Add(-24*time.Hour)))

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

	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(3))

	// no source filters
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("Sources")).ToHaveCount(0))

	// the most used tags under the search box, and each card's own tags, both
	// columns; a tap adds #tag to the search
	search := notesPanel(page).GetByRole("searchbox", playwright.LocatorGetByRoleOptions{Name: "Search notes"})
	top := notesPanel(page).GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Your most used tags"}).GetByRole("button")
	require.NoError(t, browser.Expect.Locator(top).ToHaveText([]string{"#essay", "#later", "#walk"}))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "bravo idea").GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Tags of the note"}).
		GetByRole("button")).ToHaveText([]string{"#walk", "#later"}))

	require.NoError(t, pickerCard(page, "bravo idea").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Search #later"}).Click())
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#later"))
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "bravo idea")).ToBeVisible())

	// a tag already in the search is not added twice
	require.NoError(t, notesPanel(page).GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Your most used tags"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Search #walk"}).Click())
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#later #walk"))
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, notesPanel(page).GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Your most used tags"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Search #walk"}).Click())
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#later #walk"))

	// #tag includes, -#tag leaves out, words search the text
	searchNotes(t, page, "#essay")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "bravo idea")).ToHaveCount(0))

	searchNotes(t, page, "-#essay")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "bravo idea")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "alpha idea")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "only way out")).ToHaveCount(0))

	searchNotes(t, page, "#essay alpha")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "alpha idea")).ToBeVisible())

	searchNotes(t, page, "#essay")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(2))

	insert := func(card string) playwright.Locator {
		return pickerCard(page, card).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Insert", Exact: new(true)})
	}
	appendBtn := func(card string) playwright.Locator {
		return pickerCard(page, card).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Append", Exact: new(true)})
	}

	// Insert goes in at the cursor, and saves at once
	_, err = body.Evaluate("el => el.setSelectionRange(5, 5)", nil)
	require.NoError(t, err)
	require.NoError(t, insert("alpha idea").Click())
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue("Start\n\nalpha idea\n\nEnd"))
	savedBody(t, app, postID, "Start\n\nalpha idea\n\nEnd")

	// Append goes to the end, a quote with its attribution line
	require.NoError(t, appendBtn("only way out").Click())
	want := "Start\n\nalpha idea\n\nEnd\n\n> The only way out is through.\n\n— *North of Boston*\n"
	require.NoError(t, browser.Expect.Locator(body).ToHaveValue(want))
	savedBody(t, app, postID, strings.TrimSpace(want))

	// the notes count as used
	store := repo.New(app.DB)

	require.Eventually(t, func() bool {
		a, errA := store.NotePostLinkCount(ctx, alpha.ID)
		q, errQ := store.NotePostLinkCount(ctx, quote.ID)

		return errA == nil && errQ == nil && a == 1 && q == 1
	}, 10*time.Second, 200*time.Millisecond)

	// the note's own form has no sidebar: it is the post editor's
	_, err = page.Goto("/controls/notes/" + alpha.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("toolbar", playwright.PageGetByRoleOptions{Name: "Formatting"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("tab")).ToHaveCount(0))
}

// Each card has one row of small icon buttons with a label and a title, and
// Edit opens the note's edit page in another tab.
func TestNotesPicker_IconButtonsAndEditLink(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	note := mustNote(t, app, user.ID, factory.WithNoteBody("only note"))
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)
	openNotes(t, page)

	card := pickerCard(page, "only note")
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())

	var ys []float64

	for _, c := range []struct {
		role, name string
	}{{"button", "Insert"}, {"button", "Append"}, {"link", "Edit"}, {"button", "Delete"}} {
		ico := card.GetByRole(playwright.AriaRole(c.role), playwright.LocatorGetByRoleOptions{Name: c.name, Exact: new(true)})
		require.NoError(t, browser.Expect.Locator(ico).ToBeVisible(), c.name)

		title, err := ico.GetAttribute("title")
		require.NoError(t, err)
		require.NotEmpty(t, title, c.name+" has a title")

		// a small icon, not a text button
		svg, err := ico.Locator("svg").BoundingBox()
		require.NoError(t, err)
		require.InDelta(t, 16.0, svg.Width, 0.5, c.name)
		require.InDelta(t, 16.0, svg.Height, 0.5, c.name)

		box, err := ico.BoundingBox()
		require.NoError(t, err)
		require.LessOrEqual(t, box.Width, 32.0, c.name)

		ys = append(ys, box.Y)
	}

	// in one row
	for _, y := range ys {
		require.InDelta(t, ys[0], y, 1.0)
	}

	edit := card.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Edit", Exact: new(true)})
	require.NoError(t, browser.Expect.Locator(edit).ToHaveAttribute("href", "/controls/notes/"+note.ID+"/edit"))
	require.NoError(t, browser.Expect.Locator(edit).ToHaveAttribute("target", "_blank"))
}

// Delete asks with the note's delete prompt; accepting removes the card and
// the note, dismissing keeps both.
func TestNotesPicker_Delete(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	ctx := context.Background()
	keep := mustNote(t, app, user.ID, factory.WithNoteBody("keep me"), factory.CapturedAt(time.Now().Add(-time.Hour)))
	gone := mustNote(t, app, user.ID, factory.WithNoteBody("delete me"))
	page := browser.Page(t, app, browser.As(user))

	accept := make(chan bool, 1)
	asked := make(chan string, 2)
	page.OnDialog(func(d playwright.Dialog) {
		asked <- d.Message()

		if <-accept {
			_ = d.Accept()
		} else {
			_ = d.Dismiss()
		}
	})

	_, err := page.Goto("/write")
	require.NoError(t, err)
	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(2))

	del := pickerCard(page, "delete me").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete", Exact: new(true)})
	store := repo.New(app.DB)

	// dismissed: the card and the note stay
	accept <- false

	require.NoError(t, del.Click())
	require.Contains(t, <-asked, "Delete this note")

	// settled: the panel loaded again from the server still has both notes
	searchNotes(t, page, "delete")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	searchNotes(t, page, "")
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(2))

	_, err = store.NoteWithRelations(ctx, gone.ID)
	require.NoError(t, err)

	// accepted: both go
	accept <- true

	require.NoError(t, del.Click())
	require.Contains(t, <-asked, "Delete this note")

	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "keep me")).ToBeVisible())

	require.Eventually(t, func() bool {
		_, err := store.NoteWithRelations(ctx, gone.ID)

		return errors.Is(err, repo.ErrNotFound)
	}, 10*time.Second, 200*time.Millisecond)

	_, err = store.NoteWithRelations(ctx, keep.ID)
	require.NoError(t, err)
}

// The sidebar has the tabs Checks and Notes and collapses to a slim strip
// that belongs to the editor box: level with its top, as tall as it, drawn
// like it, with a button per tab that opens the sidebar on that tab. The
// choice is remembered after a reload.
func TestEditorSidebar_TabsCollapseAndRemember(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteBody("a note"))
	var picked atomic.Int32

	page := browser.Page(t, app, browser.As(user))
	page.OnRequest(func(r playwright.Request) {
		if strings.Contains(r.URL(), "/controls/notes/picker") {
			picked.Add(1)
		}
	})

	post, err := factory.Post(context.Background(), app.DB, user.ID, factory.WithSubject("Cats"), factory.WithBody("text"))
	require.NoError(t, err)
	_, err = page.Goto("/posts/" + post.ID + "/edit")
	require.NoError(t, err)

	// a fresh browser starts with the strip, which asks for no notes
	showChecks, showNotes := sideToggle(page, "Show Checks"), sideToggle(page, "Show Notes")
	require.NoError(t, browser.Expect.Locator(showNotes).ToBeVisible())
	require.NoError(t, page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle}))
	require.Zero(t, picked.Load())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Checks")).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(sideToggle(page, "Collapse the sidebar")).ToBeHidden())

	// no Notes button in the toolbar
	toolbar := page.GetByRole("toolbar", playwright.PageGetByRoleOptions{Name: "Formatting"})
	require.NoError(t, browser.Expect.Locator(toolbar.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Notes"})).ToHaveCount(0))

	requireStripBesideBox(t, page)

	// each button is an icon with a title, the two stacked
	var ys []float64

	for _, b := range []playwright.Locator{showChecks, showNotes} {
		require.NoError(t, browser.Expect.Locator(b).ToHaveAttribute("title", regexp.MustCompile(`.+`)))
		require.NoError(t, browser.Expect.Locator(b).ToHaveAttribute("aria-expanded", "false"))
		svg, err := b.Locator("svg").BoundingBox()
		require.NoError(t, err)
		require.InDelta(t, 16.0, svg.Width, 0.5)

		box, err := b.BoundingBox()
		require.NoError(t, err)
		ys = append(ys, box.Y)
	}

	require.Greater(t, ys[1], ys[0], "Notes is under Checks")

	// Checks opens the sidebar on Checks, which loads no notes
	require.NoError(t, showChecks.Click())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Checks")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(checksPanel(page)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(notesPanel(page)).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(strip(page)).ToBeHidden())
	require.Zero(t, picked.Load(), "the Checks tab loads no notes")

	// collapse, then Notes opens it on Notes
	body := page.GetByPlaceholder("Your post goes there")
	before, err := body.BoundingBox()
	require.NoError(t, err)

	require.NoError(t, sideToggle(page, "Collapse the sidebar").Click())
	require.NoError(t, browser.Expect.Locator(checksPanel(page)).ToBeHidden())

	after, err := body.BoundingBox()
	require.NoError(t, err)
	require.Greater(t, after.Width, before.Width+200, "the editor takes the room of the sidebar")
	requireStripBesideBox(t, page)

	require.NoError(t, showNotes.Click())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "a note")).ToBeVisible())
	require.Positive(t, picked.Load())
	require.NoError(t, browser.Expect.Locator(checksPanel(page)).ToBeHidden())

	// remembered after a reload, open on Notes
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(sideToggle(page, "Collapse the sidebar")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "a note")).ToBeVisible())

	// and collapsed after a reload
	require.NoError(t, sideToggle(page, "Collapse the sidebar").Click())
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(showNotes).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToBeHidden())

	// the strip follows the box when the box grows
	_, err = body.Evaluate("el => { el.rows = 30 }", nil)
	require.NoError(t, err)
	requireStripBesideBox(t, page)

	// the sidebar is per browser, not per post
	require.NoError(t, showNotes.Click())
	_, err = page.Goto("/write")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToHaveAttribute("aria-selected", "true"))
}

// Without proofreading the sidebar has only Notes, and its strip one button.
func TestEditorSidebar_NotesOnlyWithoutProofread(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/write")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(strip(page).GetByRole("button")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(sideToggle(page, "Show Checks")).ToHaveCount(0))
	requireStripBesideBox(t, page)

	// the first autosave pushes the edit page's URL; the strip stays in place
	require.NoError(t, page.GetByPlaceholder("Subject").Fill("Strip"))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("text"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))
	requireStripBesideBox(t, page)

	require.NoError(t, sideToggle(page, "Show Notes").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("tab")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToHaveAttribute("aria-selected", "true"))
}

// The strip is drawn like the editor box, in light and dark mode alike.
func TestEditorSidebar_StripLooksLikeTheBox(t *testing.T) {
	t.Parallel()

	for _, scheme := range []*playwright.ColorScheme{playwright.ColorSchemeLight, playwright.ColorSchemeDark} {
		t.Run(string(*scheme), func(t *testing.T) {
			t.Parallel()

			app := e2e.Start(t, e2e.WithRealAssets())
			user := browser.NewUser(t, app)
			page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
				o.ColorScheme = scheme
			}))

			_, err := page.Goto("/write")
			require.NoError(t, err)
			require.NoError(t, browser.Expect.Locator(sideToggle(page, "Show Notes")).ToBeVisible())

			style := func(sel string) []string {
				v, err := page.Locator(sel).First().Evaluate(`el => { const s = getComputedStyle(el);
					return [s.backgroundColor, s.borderTopColor, s.borderTopWidth, s.borderTopLeftRadius] }`, nil)
				require.NoError(t, err)

				out := []string{}
				for _, x := range v.([]any) {
					out = append(out, x.(string))
				}

				return out
			}

			box, side := style(".ed-box"), style(".es")
			require.Equal(t, box, side)

			bg, err := page.Locator("body").Evaluate(`el => getComputedStyle(el).backgroundColor`, nil)
			require.NoError(t, err)
			require.NotEqual(t, bg, box[0], "the strip stands out from the page like the box does")
		})
	}
}

// requireStripBesideBox checks that the collapsed sidebar is level with the
// editor box's top and as tall as the box, right of it.
func requireStripBesideBox(t *testing.T, page playwright.Page) {
	t.Helper()

	box := page.Locator(".ed-box").First()
	side := page.Locator(".es")

	require.Eventually(t, func() bool {
		b, err1 := box.BoundingBox()
		s, err2 := side.BoundingBox()
		if err1 != nil || err2 != nil || b == nil || s == nil {
			return false
		}

		return math.Abs(b.Y-s.Y) < 1 && math.Abs(b.Height-s.Height) < 1 && s.X > b.X+b.Width && s.Width < 56
	}, 5*time.Second, 50*time.Millisecond, "the strip runs beside the editor box")
}

// Check grammar and Style review show the Checks tab, from the Notes tab and
// from the collapsed strip.
func TestEditorSidebar_GrammarButtonOpensChecks(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteBody("a note"))
	page, _ := proofreadEditor(t, app, user, "Cats", "wm-br-apply: the cta sat on the mat.")
	suggestions := page.GetByRole("complementary", playwright.PageGetByRoleOptions{Name: "Suggestions"})

	// from the Notes tab
	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeHidden())

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Checks")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(notesPanel(page)).ToBeHidden())

	// from the collapsed strip
	openNotes(t, page)
	require.NoError(t, sideToggle(page, "Collapse the sidebar").Click())
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeHidden())
	require.NoError(t, browser.Expect.Locator(sideToggle(page, "Show Checks")).ToBeVisible())

	require.NoError(t, button(page, "Check grammar").Click())
	require.NoError(t, browser.Expect.Locator(sideToggle(page, "Collapse the sidebar")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(sideTab(page, "Checks")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(suggestions).ToBeVisible())
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
	require.NoError(t, notesPanel(page).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Insert", Exact: new(true)}).Click())
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

// A save that fails validation draws the editor again; the sidebar comes back
// as it was, on the Notes tab with its notes.
func TestNotesPicker_SidebarSurvivesAFailedSave(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteBody("a note"))
	page := browser.Page(t, app, browser.As(user), browser.Allow(`400|422`))

	_, err := page.Goto("/write")
	require.NoError(t, err)
	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "a note")).ToBeVisible())

	// mark the sidebar that is on the page now: the redraw must replace it
	_, err = page.Locator("#es-notes").Evaluate("el => el.setAttribute('data-old', '1')", nil)
	require.NoError(t, err)

	require.NoError(t, page.GetByPlaceholder("Subject").Fill(strings.Repeat("long ", posts.DefaultPostSubjectMaxLength)))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("text"))
	require.NoError(t, browser.Expect.Locator(page.Locator(".field-error")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.Locator("[data-old]")).ToHaveCount(0), "the editor was drawn again")

	require.NoError(t, browser.Expect.Locator(sideTab(page, "Notes")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "a note")).ToBeVisible())
}
