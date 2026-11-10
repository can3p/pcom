//go:build browser

// Everything an author does on the notes pages: the search box with its
// chips, every tag of a note on show, selections, sources added, renamed and
// merged in place, the edit page with its source, quick capture with tags, a
// source, photos and recordings, and each delete with its confirmation.
package browser_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	notesvc "github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// noteDialogs answers the confirms and prompts of a page and remembers what
// they said.
type noteDialogs struct {
	mu     sync.Mutex
	accept bool
	answer string
	msgs   []string
}

func watchDialogs(page playwright.Page) *noteDialogs {
	d := &noteDialogs{accept: true}

	page.OnDialog(func(dlg playwright.Dialog) {
		d.mu.Lock()
		d.msgs = append(d.msgs, dlg.Message())
		accept, answer := d.accept, d.answer
		d.mu.Unlock()

		if accept {
			_ = dlg.Accept(answer)
		} else {
			_ = dlg.Dismiss()
		}
	})

	return d
}

// reply sets how the next dialogs are answered.
func (d *noteDialogs) reply(accept bool, answer string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.accept, d.answer = accept, answer
}

func (d *noteDialogs) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.msgs = nil
}

func (d *noteDialogs) last() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.msgs) == 0 {
		return ""
	}

	return d.msgs[len(d.msgs)-1]
}

func noteCard(page playwright.Page, text string) playwright.Locator {
	return page.GetByTestId("note").Filter(playwright.LocatorFilterOptions{HasText: text})
}

func noteIn(t *testing.T, app *e2e.App, id string) *model.Note {
	t.Helper()

	n, err := repo.New(app.DB).NoteByID(context.Background(), id)
	require.NoError(t, err)

	return n
}

func noteGone(t *testing.T, app *e2e.App, id string) bool {
	t.Helper()

	_, err := repo.New(app.DB).NoteByID(context.Background(), id)

	return err != nil
}

func mustNote(t *testing.T, app *e2e.App, userID string, opts ...factory.NoteOpt) *model.Note {
	t.Helper()

	n, err := factory.Note(context.Background(), app.DB, userID, opts...)
	require.NoError(t, err)

	return n
}

// The search box takes #tags, -#tags and words; each part shows as a chip
// whose × drops it, a tag on a note or under the box adds it to the search.
func TestNotes_Search(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	now := time.Now().UTC()

	mustNote(t, app, user.ID, factory.WithNoteBody("alpha note"), factory.WithNoteTags("essay"), factory.CapturedAt(now.Add(-4*time.Hour)))
	mustNote(t, app, user.ID, factory.WithNoteBody("bravo note"), factory.WithNoteTags("essay", "done"), factory.CapturedAt(now.Add(-3*time.Hour)))
	mustNote(t, app, user.ID, factory.WithNoteBody("charlie note"), factory.WithNoteTags("walk"), factory.CapturedAt(now.Add(-2*time.Hour)))
	mustNote(t, app, user.ID, factory.WithNoteBody("delta"), factory.CapturedAt(now.Add(-time.Hour)))

	page := browser.Page(t, app, browser.As(user))
	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	notes := page.GetByTestId("note")
	filters := page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Active filters"})
	search := page.GetByRole("searchbox", playwright.PageGetByRoleOptions{Name: "Search notes"})
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(4))
	require.NoError(t, browser.Expect.Locator(filters).ToHaveCount(0))

	require.NoError(t, search.Fill("#Essay -#done note"))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "alpha note")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(filters.GetByRole("listitem")).ToHaveText([]string{"#essay ×", "not #done ×", "“note” ×"}))

	// × on the excluded tag brings its notes back, the rest of the search stays
	require.NoError(t, filters.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Remove not #done"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "bravo note")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#essay note"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`[?&]q=%23essay\+note$`)))

	require.NoError(t, filters.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Remove #essay"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(3))
	require.NoError(t, filters.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Remove “note”"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(4))
	require.NoError(t, browser.Expect.Locator(filters).ToHaveCount(0))

	// a tag on a note adds it to the search on show
	require.NoError(t, search.Fill("note"))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(3))
	require.NoError(t, noteCard(page, "charlie note").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#walk"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#walk note"))

	// so does a tag under the box, to the search on show; an excluded tag reads "not"
	require.NoError(t, search.Fill("-#walk"))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Locator(filters.GetByRole("listitem")).ToHaveText([]string{"not #walk ×"}))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(3))
	require.NoError(t, page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Your tags"}).
		GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Search #done"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "bravo note")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(search).ToHaveValue("#done -#walk"))
}

// Every note shows all its tags, set by hand and written in the text; a
// selection of notes is tagged at once, and the new tags show.
func TestNotes_TagsShowAndSelection(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	now := time.Now().UTC()

	mixed := mustNote(t, app, user.ID, factory.WithNoteBody("old thoughts #inline"), factory.WithNoteTags("essay"),
		factory.WithNoteTextTags("inline"), factory.CapturedAt(now.Add(-72*time.Hour)))

	var picked []*model.Note
	for i := range 3 {
		picked = append(picked, mustNote(t, app, user.ID,
			factory.WithNoteBody(fmt.Sprintf("picked %d", i)), factory.CapturedAt(now.Add(-time.Duration(30-i*5)*time.Minute))))
	}

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	tagsOf := func(text string) playwright.Locator {
		return noteCard(page, text).GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Tags of the note"}).GetByRole("listitem")
	}

	// the tag set by hand and the one written in the text
	require.NoError(t, browser.Expect.Locator(tagsOf("old thoughts")).ToHaveText([]string{"#essay", "#inline (written in the text)"}))
	require.Equal(t, []string{"inline"}, noteIn(t, app, mixed.ID).TextTags)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Views"})).ToHaveCount(0))

	for _, n := range picked {
		require.NoError(t, noteCard(page, n.Body).GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: "Select this note"}).Check())
	}

	require.NoError(t, browser.Expect.Locator(page.GetByText("3 selected")).ToBeVisible())

	dialogs.reply(true, "trip, photos")
	require.NoError(t, page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Selected notes"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Tag"}).Click())

	// the action's tags show on every note it tagged
	for _, n := range picked {
		require.NoError(t, browser.Expect.Locator(tagsOf(n.Body)).ToHaveText([]string{"#trip", "#photos"}))
		require.ElementsMatch(t, []string{"trip", "photos"}, noteIn(t, app, n.ID).Tags)
	}
}

// Delete asks first, saying what goes with the note; a selection says how
// many notes and recordings.
func TestNotes_DeleteFromListAndSelection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	now := time.Now().UTC()

	audio, err := factory.NoteRecording(ctx, app.DB, user.ID)
	require.NoError(t, err)

	spoken := mustNote(t, app, user.ID, factory.WithNoteBody("spoken note"), factory.WithRecording(audio.ID), factory.CapturedAt(now.Add(-4*time.Hour)))
	typed := mustNote(t, app, user.ID, factory.WithNoteBody("typed note"), factory.CapturedAt(now.Add(-3*time.Hour)))
	keep := mustNote(t, app, user.ID, factory.WithNoteBody("kept note"), factory.CapturedAt(now.Add(-2*time.Hour)))
	lone := mustNote(t, app, user.ID, factory.WithNoteBody("lone note"), factory.CapturedAt(now.Add(-time.Hour)))

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)

	del := noteCard(page, "lone note").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"})

	dialogs.reply(false, "")
	require.NoError(t, del.Click())
	require.Eventually(t, func() bool { return dialogs.last() != "" }, 5*time.Second, 50*time.Millisecond)
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "lone note")).ToBeVisible())
	require.False(t, noteGone(t, app, lone.ID))

	dialogs.reply(true, "")
	require.NoError(t, del.Click())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "lone note")).ToHaveCount(0))
	require.True(t, noteGone(t, app, lone.ID))

	require.NoError(t, noteCard(page, "spoken note").GetByRole("checkbox").Check())
	require.NoError(t, noteCard(page, "typed note").GetByRole("checkbox").Check())
	require.NoError(t, browser.Expect.Locator(page.GetByText("2 selected")).ToBeVisible())

	require.NoError(t, page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Selected notes"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(1))
	require.Equal(t, "Delete 2 notes and 1 recording? This can't be undone.", dialogs.last())
	require.True(t, noteGone(t, app, spoken.ID))
	require.True(t, noteGone(t, app, typed.ID))
	require.False(t, noteGone(t, app, keep.ID))
}

// Sources are a view of the list. The management screen renames a tag, which
// merges it into one that exists; adds a source; and edits a source in place:
// the pencil swaps in a form with its title, Cancel puts the row back, Save
// renames it in place, and a title another source has merges the two.
func TestNotes_TagsAndSourcesScreen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	first, err := factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Walden"))
	require.NoError(t, err)

	second, err := factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Cape Cod"))
	require.NoError(t, err)

	quote := mustNote(t, app, user.ID, factory.WithNoteBody("quote one"), factory.FromSource(first.ID))
	other := mustNote(t, app, user.ID, factory.WithNoteBody("quote two"), factory.FromSource(second.ID), factory.WithNoteTags("essays"))
	mustNote(t, app, user.ID, factory.WithNoteBody("no source"), factory.WithNoteTags("essay"))

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)
	store := repo.New(app.DB)

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Sources"}).
		GetByRole("link", playwright.LocatorGetByRoleOptions{Name: first.Title}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "quote one")).ToBeVisible())

	_, err = page.Goto("/controls/notes/manage")
	require.NoError(t, err)

	// "essays" renamed to the existing "essay" merges the two
	dialogs.reply(true, "essay")
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Rename #essays"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("tag-row")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("tag-row")).ToContainText("2 notes"))
	require.Equal(t, []string{"essay"}, noteIn(t, app, other.ID).Tags)

	rows := page.GetByTestId("source-row")
	require.NoError(t, browser.Expect.Locator(rows).ToHaveText([]string{"Cape Cod 1 note", "Walden 1 note"}))

	// Add source: an empty title is an error under the field, a title adds it
	add := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add source"})
	newTitle := page.GetByLabel("New source")
	require.NoError(t, add.Click())
	require.NoError(t, browser.Expect.Locator(page.Locator("#note-source-add .field-error")).ToHaveText("A source needs a title."))
	require.NoError(t, browser.Expect.Locator(newTitle).ToHaveAttribute("aria-invalid", "true"))

	require.NoError(t, newTitle.Fill(strings.Repeat("x", notesvc.DefaultSourceTitleMaxLength+1)))
	require.NoError(t, add.Click())
	require.NoError(t, browser.Expect.Locator(page.Locator("#note-source-add .field-error")).ToContainText("at most"))

	require.NoError(t, newTitle.Fill("  The   Maine Woods "))
	require.NoError(t, add.Click())
	require.NoError(t, browser.Expect.Locator(rows).ToHaveText([]string{"Cape Cod 1 note", "The Maine Woods 0 notes", "Walden 1 note"}))

	// Edit swaps in the form with the title; Cancel puts the row back
	edit := func(title string) {
		t.Helper()
		require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Edit " + title, Exact: new(true)}).Click())
	}
	editTitle := page.GetByTestId("source-row").GetByLabel("Title", playwright.LocatorGetByLabelOptions{Exact: new(true)})

	edit("Walden")
	require.NoError(t, browser.Expect.Locator(editTitle).ToHaveValue("Walden"))
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Cancel"}).Click())
	require.NoError(t, browser.Expect.Locator(editTitle).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(rows).ToHaveText([]string{"Cape Cod 1 note", "The Maine Woods 0 notes", "Walden 1 note"}))

	// Save renames in place, an empty title is an error under the field
	edit("Walden")
	require.NoError(t, editTitle.Fill(""))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("source-row").Locator(".field-error")).ToHaveText("A source needs a title."))

	require.NoError(t, editTitle.Fill("Walden Pond"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
	saved := rows.Filter(playwright.LocatorFilterOptions{HasText: "Walden Pond"})
	require.NoError(t, browser.Expect.Locator(saved.GetByRole("status")).ToContainText("Saved"))
	require.Equal(t, "Walden Pond", testutil.Must(store.NoteSourceByID(ctx, first.ID))(t).Title)

	// a title another source has merges them: the notes move, one row goes
	edit("Cape Cod")
	require.NoError(t, editTitle.Fill("walden pond"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(rows).ToHaveText([]string{"The Maine Woods 0 notes", "Walden Pond 2 notes"}))
	require.Equal(t, first.ID, *noteIn(t, app, other.ID).SourceID)
	require.Equal(t, first.ID, *noteIn(t, app, quote.ID).SourceID)
}

// The edit page saves in place, takes #words as tags and deletes the note
// after asking.
func TestNotes_EditPage(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	note := mustNote(t, app, user.ID, factory.WithNoteBody("first draft"))

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes/" + note.ID + "/edit")
	require.NoError(t, err)

	body := page.GetByLabel("Note", playwright.PageGetByLabelOptions{Exact: new(true)})
	require.NoError(t, body.Fill("second draft #fresh"))
	require.NoError(t, page.GetByLabel("Tags").Fill("one two"))

	// no Save: the form saves by itself after the typing
	require.NoError(t, browser.Expect.Locator(page.GetByRole("status").Filter(playwright.LocatorFilterOptions{HasText: "Saved"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.Locator("#note-tag-chips li").Filter(playwright.LocatorFilterOptions{HasText: "#fresh"})).ToBeVisible())

	saved := noteIn(t, app, note.ID)
	require.Equal(t, "second draft #fresh", saved.Body)
	require.ElementsMatch(t, []string{"one", "two"}, saved.Tags)
	require.Equal(t, []string{"fresh"}, saved.TextTags)

	require.NoError(t, page.GetByLabel("Tags").Fill("bad!tag"))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Not saved")).ToBeVisible())

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes$`)))
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.True(t, noteGone(t, app, note.ID))
}

// A note typed into the capture box with Ctrl+Enter lands in the list, with
// the tags typed next to it, a recent tag tapped in and the source chosen; the
// Notes section of the controls page links to the list.
func TestNotes_QuickCaptureAndControls(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByText("No notes yet.")).ToBeVisible())

	require.NoError(t, page.GetByRole("navigation").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Notes", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes$`)))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Recent tags"})).ToHaveCount(0))

	capture := page.GetByLabel("New note")
	require.NoError(t, capture.Fill("   "))
	require.NoError(t, capture.Press("Control+Enter"))
	require.NoError(t, browser.Expect.Locator(page.GetByText("Write something first.")).ToBeVisible())

	require.NoError(t, capture.Fill("bought the book"))
	require.NoError(t, capture.Press("Control+Enter"))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "bought the book")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("New note")).ToHaveValue(""))

	_, err = page.Goto("/controls/")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "bought the book"})).ToBeVisible())

	// tags typed with the text, then a recent one tapped in, and a source chosen
	mustNote(t, app, user.ID, factory.WithNoteBody("earlier"), factory.WithNoteTags("reading"), factory.CapturedAt(time.Now().UTC().Add(-time.Hour)))
	src, err := factory.NoteSource(context.Background(), app.DB, user.ID, factory.WithSourceTitle("Walden"))
	require.NoError(t, err)

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)

	tags := page.GetByLabel("Tags", playwright.PageGetByLabelOptions{Exact: new(true)})
	require.NoError(t, page.GetByLabel("New note").Fill("a tagged thought"))
	require.NoError(t, tags.Fill("#Essay, walk"))
	recent := page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Recent tags"})
	require.NoError(t, recent.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add tag #reading"}).Click())
	require.NoError(t, recent.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add tag #reading"}).Click())
	require.NoError(t, browser.Expect.Locator(tags).ToHaveValue("#Essay walk reading"))
	source := page.GetByLabel("Source", playwright.PageGetByLabelOptions{Exact: new(true)})
	require.NoError(t, browser.Expect.Locator(source.Locator("option")).ToHaveText([]string{"No source", "Walden"}))
	_, err = source.SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Walden"}})
	require.NoError(t, err)
	require.NoError(t, tags.Press("Control+Enter"))

	card := noteCard(page, "a tagged thought")
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(card.Locator(".nt-tags li")).ToHaveText([]string{"#essay", "#walk", "#reading"}))
	require.NoError(t, browser.Expect.Locator(card.GetByTestId("note-source")).ToHaveText("Walden"))

	n := userNotes(t, app, user.ID)[0]
	require.Equal(t, "a tagged thought", n.Body)
	require.Equal(t, []string{"essay", "walk", "reading"}, n.Tags)
	require.Equal(t, src.ID, *n.SourceID)
}

// The capture box and the edit page have the markdown toolbar: a photo picked
// with it is uploaded and goes into the text as an image.
func TestNotes_PhotoFromTheToolbar(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	capture := page.GetByLabel("New note")
	image := regexp.MustCompile(`^a page\n!\[wide\.png\]\([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.png\)$`)

	require.NoError(t, capture.Fill("a page"))
	chooser, err := page.ExpectFileChooser(func() error {
		return page.Locator(".note-capture").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Upload images", Exact: new(true)}).Click()
	})
	require.NoError(t, err)
	require.NoError(t, chooser.SetFiles([]playwright.InputFile{{Name: "wide.png", MimeType: "image/png", Buffer: widePNG(t)}}))
	require.NoError(t, browser.Expect.Locator(capture).ToHaveValue(image))
	require.NoError(t, capture.Press("Control+Enter"))

	card := noteCard(page, "a page")
	require.NoError(t, browser.Expect.Locator(card.GetByRole("img", playwright.LocatorGetByRoleOptions{Name: "wide.png"})).ToBeVisible())
	n := userNotes(t, app, user.ID)[0]
	require.Regexp(t, image, n.Body)

	_, err = page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)

	body := page.GetByLabel("Note", playwright.PageGetByLabelOptions{Exact: new(true)})
	require.NoError(t, body.Fill("edited"))
	chooser, err = page.ExpectFileChooser(func() error {
		return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Upload images", Exact: new(true)}).Click()
	})
	require.NoError(t, err)
	require.NoError(t, chooser.SetFiles([]playwright.InputFile{{Name: "wide.png", MimeType: "image/png", Buffer: widePNG(t)}}))
	require.Eventually(t, func() bool {
		return regexp.MustCompile(`^edited\n!\[wide\.png\]\([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.png\)$`).MatchString(noteIn(t, app, n.ID).Body)
	}, 10*time.Second, 100*time.Millisecond)
}

// "Upload recording" makes a note of a recording from the device, which the
// list shows transcribing; "Add recording" on the edit page of a note without
// one attaches it.
func TestNotes_UploadRecording(t *testing.T) {
	t.Parallel()

	app := sttApp(t)
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user), browser.Allow(`status of 400`))
	memo := playwright.InputFile{Name: "memo.ogg", MimeType: "audio/ogg", Buffer: append([]byte("OggS"), make([]byte, 256)...)}

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Upload recording").SetInputFiles([]playwright.InputFile{memo}))

	notes := page.GetByTestId("note")
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(notes.GetByTestId("note-status")).ToHaveText("Transcribing…"))

	n := waitNotes(t, app, user.ID, 1)[0]
	require.NotNil(t, n.RecordingID)
	require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
	require.Equal(t, 1, testutil.Must(factory.CountNoteJobs(context.Background(), app.DB, notesvc.TranscribeArgs{}.Kind(), user.ID))(t))

	// a refusal shows next to the button
	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Upload recording").SetInputFiles([]playwright.InputFile{{Name: "x.ogg", MimeType: "audio/ogg", Buffer: []byte("not audio")}}))
	require.NoError(t, browser.Expect.Locator(page.Locator(".note-capture .status.err")).ToContainText("recording"))
	require.Len(t, userNotes(t, app, user.ID), 1)

	typed := mustNote(t, app, user.ID, factory.WithNoteBody("typed first"))

	_, err = page.Goto("/controls/notes/" + typed.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Add recording").SetInputFiles([]playwright.InputFile{memo}))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-status")).ToContainText("Transcribing…"))
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Add recording")).ToHaveCount(0))
	require.NotNil(t, noteIn(t, app, typed.ID).RecordingID)
	require.Len(t, userNotes(t, app, user.ID), 2)
}

// The edit page's Source is a choice of the author's sources, saved with the
// note by itself; the list shows it, and No source takes it away.
func TestNotes_EditSource(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	src, err := factory.NoteSource(context.Background(), app.DB, user.ID, factory.WithSourceTitle("Walden"))
	require.NoError(t, err)
	_, err = factory.NoteSource(context.Background(), app.DB, user.ID, factory.WithSourceTitle("Cape Cod"))
	require.NoError(t, err)
	n := mustNote(t, app, user.ID, factory.WithNoteBody("loose quote"))

	page := browser.Page(t, app, browser.As(user))
	source := page.GetByLabel("Source", playwright.PageGetByLabelOptions{Exact: new(true)})
	saved := page.GetByRole("status").Filter(playwright.LocatorFilterOptions{HasText: "Saved"})

	_, err = page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(source.Locator("option")).ToHaveText([]string{"No source", "Cape Cod", "Walden"}))
	require.NoError(t, browser.Expect.Locator(source).ToHaveValue(""))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("form", playwright.PageGetByRoleOptions{Name: "Source"})).ToHaveCount(0))

	// choosing saves by itself, with the note
	_, err = source.SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Walden"}})
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(saved).ToBeVisible())
	require.Eventually(t, func() bool {
		id := noteIn(t, app, n.ID).SourceID
		return id != nil && *id == src.ID
	}, 10*time.Second, 100*time.Millisecond)

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(noteCard(page, "loose quote").GetByTestId("note-source")).ToHaveText("Walden"))

	_, err = page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(source).ToHaveValue(src.ID))
	_, err = source.SelectOption(playwright.SelectOptionValues{Labels: &[]string{"No source"}})
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(saved).ToBeVisible())
	require.Eventually(t, func() bool { return noteIn(t, app, n.ID).SourceID == nil }, 10*time.Second, 100*time.Millisecond)
}

// A note's Edit and Delete are icon buttons named for what they do.
func TestNotes_IconActions(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	n := mustNote(t, app, user.ID, factory.WithNoteBody("iconic"))

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	card := noteCard(page, "iconic")
	edit := card.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Edit note", Exact: new(true)})
	del := card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete note", Exact: new(true)})
	require.NoError(t, browser.Expect.Locator(edit).ToHaveAttribute("aria-label", "Edit note"))
	require.NoError(t, browser.Expect.Locator(del).ToHaveAttribute("aria-label", "Delete note"))
	require.NoError(t, browser.Expect.Locator(edit).ToHaveAttribute("title", "Edit note"))
	require.NoError(t, browser.Expect.Locator(del).ToHaveAttribute("title", "Delete note"))
	require.NoError(t, browser.Expect.Locator(edit).ToHaveText(""))
	require.NoError(t, browser.Expect.Locator(edit.Locator("svg")).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(del.Locator("svg")).ToHaveCount(1))

	require.NoError(t, edit.Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes/`+n.ID+`/edit$`)))

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, del.Click())
	require.NoError(t, browser.Expect.Locator(card).ToHaveCount(0))
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.True(t, noteGone(t, app, n.ID))
}

// "Load more" appends the older notes in place: one page, one heading, and
// the notes of the first page stay.
func TestNotes_LoadMore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	now := time.Now().UTC().Add(-24 * time.Hour)

	const total = 32

	for i := range total {
		mustNote(t, app, user.ID, factory.WithNoteBody(fmt.Sprintf("page note %02d", i)), factory.CapturedAt(now.Add(time.Duration(i)*time.Hour)))
	}

	page := browser.Page(t, app, browser.As(user))
	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	notes := page.GetByTestId("note")
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(30))
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Load more"}).Click())
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(total))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Notes", Exact: new(true)})).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "page note 31")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "page note 00")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Load more"})).ToHaveCount(0))
}

// A note's photos show in a strip, and the confirmations of Delete, in the
// list and on the edit page, count the recording, the photos and the posts the
// text went into.
func TestNotes_PhotosAndDeleteConfirmation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	audio, err := factory.NoteRecording(ctx, app.DB, user.ID)
	require.NoError(t, err)

	note := mustNote(t, app, user.ID, factory.WithNoteBody("with photos"), factory.WithRecording(audio.ID))

	post, err := factory.Post(ctx, app.DB, user.ID)
	require.NoError(t, err)
	_, err = factory.NotePostLink(ctx, app.DB, note.ID, post.ID)
	require.NoError(t, err)

	var files []string

	for i := range 2 {
		up, err := factory.MediaUpload(ctx, app.DB, user.ID)
		require.NoError(t, err)

		_, err = factory.NoteImage(ctx, app.DB, note.ID, up.ID, i)
		require.NoError(t, err)

		files = append(files, up.UploadedFname)
	}

	// the factory's uploads have no file behind them
	page := browser.Page(t, app, browser.As(user), browser.Allow(`user-media|Failed to load resource`))
	dialogs := watchDialogs(page)
	dialogs.reply(false, "")

	const prompt = "Delete this note, its recording and 2 photos? This can't be undone. The text already inserted into posts stays there."

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)

	photos := noteCard(page, "with photos").GetByAltText("Photo of the note")
	require.NoError(t, browser.Expect.Locator(photos).ToHaveCount(2))

	for i, f := range files {
		require.NoError(t, browser.Expect.Locator(photos.Nth(i)).ToHaveAttribute("src", regexp.MustCompile(regexp.QuoteMeta(f)+`$`)))
	}

	require.NoError(t, noteCard(page, "with photos").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click())
	require.Eventually(t, func() bool { return dialogs.last() == prompt }, 5*time.Second, 50*time.Millisecond)

	dialogs.reset()

	_, err = page.Goto("/controls/notes/" + note.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete"}).Click())
	require.Eventually(t, func() bool { return dialogs.last() == prompt }, 5*time.Second, 50*time.Millisecond)
	require.False(t, noteGone(t, app, note.ID))
}
