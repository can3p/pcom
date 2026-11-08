//go:build browser

// Everything an author does on the notes pages: the search box with its
// chips, the Inbox with its suggestions and selections, sources, renaming and
// merging, the edit page with its source, quick capture with tags, photos and
// recordings, and each delete with its confirmation.
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

// The Inbox holds the untagged notes and offers the tags that fit one tap
// away; a selection of notes is tagged at once.
func TestNotes_InboxSuggestionsAndSelection(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	now := time.Now().UTC()

	mustNote(t, app, user.ID, factory.WithNoteBody("old thoughts"), factory.WithNoteTags("essay"), factory.CapturedAt(now.Add(-72*time.Hour)))
	walking := mustNote(t, app, user.ID, factory.WithNoteBody("a walking essay about streets"), factory.CapturedAt(now.Add(-48*time.Hour)))

	var picked []*model.Note
	for i := range 3 {
		picked = append(picked, mustNote(t, app, user.ID,
			factory.WithNoteBody(fmt.Sprintf("picked %d", i)), factory.CapturedAt(now.Add(-time.Duration(30-i*5)*time.Minute))))
	}

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	require.NoError(t, page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Views"}).
		GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Inbox"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`inbox=1`)))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "old thoughts")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "a walking essay")).ToBeVisible())

	suggestion := noteCard(page, "a walking essay").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Tag this note #essay"})
	require.NoError(t, suggestion.Click())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "a walking essay")).ToHaveCount(0))
	require.Equal(t, []string{"essay"}, noteIn(t, app, walking.ID).Tags)

	for _, n := range picked {
		require.NoError(t, noteCard(page, n.Body).GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: "Select this note"}).Check())
	}

	require.NoError(t, browser.Expect.Locator(page.GetByText("3 selected")).ToBeVisible())

	dialogs.reply(true, "trip, photos")
	require.NoError(t, page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Selected notes"}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Tag"}).Click())
	// tagged, they leave the Inbox
	require.NoError(t, browser.Expect.Locator(noteCard(page, "picked")).ToHaveCount(0))

	for _, n := range picked {
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

	audio, err := factory.NoteAudio(ctx, app.DB, user.ID)
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

// Sources are a view of the list; the management screen renames a tag, which
// merges it into one that exists, renames and merges sources, and sets the
// focus.
func TestNotes_SourcesRenameMergeAndFocus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	first, err := factory.NoteSource(ctx, app.DB, user.ID)
	require.NoError(t, err)

	second, err := factory.NoteSource(ctx, app.DB, user.ID)
	require.NoError(t, err)

	quote := mustNote(t, app, user.ID, factory.WithNoteBody("quote one"), factory.FromSource(first.ID))
	other := mustNote(t, app, user.ID, factory.WithNoteBody("quote two"), factory.FromSource(second.ID), factory.WithNoteTags("essays"))
	mustNote(t, app, user.ID, factory.WithNoteBody("no source"), factory.WithNoteTags("essay"))

	page := browser.Page(t, app, browser.As(user))
	dialogs := watchDialogs(page)

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

	dialogs.reply(true, "A better title")
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Rename " + first.Title}).Click())

	// merge the renamed source into the second one
	row := page.GetByTestId("source-row").Filter(playwright.LocatorFilterOptions{
		Has: page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "A better title"}),
	})
	require.NoError(t, browser.Expect.Locator(row).ToBeVisible())
	_, err = row.GetByRole("combobox").SelectOption(playwright.SelectOptionValues{Labels: &[]string{second.Title}})
	require.NoError(t, err)
	require.NoError(t, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Merge into"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("source-row")).ToHaveCount(1))
	require.Equal(t, second.ID, *noteIn(t, app, quote.ID).SourceID)

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Focus on #essay"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("tag-row")).ToContainText("notes, focus"))

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("Now writing: #essay"))

	require.NoError(t, page.GetByTestId("focus").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Stop"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))
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
// the tags typed next to it and a recent tag tapped in; the Notes section of
// the controls page counts the Inbox and links to the list.
func TestNotes_QuickCaptureAndControls(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByText("The Inbox is empty")).ToBeVisible())

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
	require.NoError(t, browser.Expect.Locator(page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "1 untagged note waits in the Inbox"})).ToBeVisible())

	// tags typed with the text, then a recent one tapped in
	mustNote(t, app, user.ID, factory.WithNoteBody("earlier"), factory.WithNoteTags("reading"), factory.CapturedAt(time.Now().UTC().Add(-time.Hour)))

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)

	tags := page.GetByLabel("Tags", playwright.PageGetByLabelOptions{Exact: new(true)})
	require.NoError(t, page.GetByLabel("New note").Fill("a tagged thought"))
	require.NoError(t, tags.Fill("#Essay, walk"))
	recent := page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Recent tags"})
	require.NoError(t, recent.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add tag #reading"}).Click())
	require.NoError(t, recent.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add tag #reading"}).Click())
	require.NoError(t, browser.Expect.Locator(tags).ToHaveValue("#Essay walk reading"))
	require.NoError(t, tags.Press("Control+Enter"))

	card := noteCard(page, "a tagged thought")
	require.NoError(t, browser.Expect.Locator(card).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(card.Locator(".nt-tags li")).ToHaveText([]string{"#essay", "#walk", "#reading"}))

	n := userNotes(t, app, user.ID)[0]
	require.Equal(t, "a tagged thought", n.Body)
	require.Equal(t, []string{"essay", "walk", "reading"}, n.Tags)
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
	require.NotNil(t, n.AudioUploadID)
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
	require.NotNil(t, noteIn(t, app, typed.ID).AudioUploadID)
	require.Len(t, userNotes(t, app, user.ID), 2)
}

// The edit page's Source: picking one of the author's sources fills its
// author, saving gives the note that source and its place in it, which the
// list shows; an empty title takes it away.
func TestNotes_EditSource(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	src, err := factory.NoteSource(context.Background(), app.DB, user.ID, factory.WithSourceAuthor("Ann Author"))
	require.NoError(t, err)
	mustNote(t, app, user.ID, factory.WithNoteBody("from the book"), factory.FromSource(src.ID))
	n := mustNote(t, app, user.ID, factory.WithNoteBody("loose quote"))
	book := &src.ID

	page := browser.Page(t, app, browser.As(user))
	form := page.GetByRole("form", playwright.PageGetByRoleOptions{Name: "Source"})
	title := form.GetByLabel("Title")
	save := form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save source"})

	_, err = page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, title.Fill(src.Title))
	require.NoError(t, browser.Expect.Locator(form.GetByLabel("Author")).ToHaveValue("Ann Author"))
	// the same book typed with other case and spacing is still that book
	require.NoError(t, title.Fill(" "+strings.ToUpper(src.Title)))
	require.NoError(t, form.GetByLabel("Author").Fill("ann  author"))
	require.NoError(t, form.GetByLabel("Where in it").Fill("p. 12"))
	require.NoError(t, save.Click())
	require.NoError(t, browser.Expect.Locator(form.GetByRole("status")).ToHaveText("Saved"))

	saved := noteIn(t, app, n.ID)
	require.Equal(t, *book, *saved.SourceID)
	require.Equal(t, "p. 12", *saved.Locator)

	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(noteCard(page, "loose quote").GetByTestId("note-source")).ToHaveText(src.Title+", Ann Author"))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "loose quote").Locator(".meta")).ToContainText("p. 12"))

	_, err = page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(title).ToHaveValue(src.Title))
	require.NoError(t, title.Fill(""))
	require.NoError(t, form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save source"}).Click())
	require.NoError(t, browser.Expect.Locator(form.GetByRole("status")).ToHaveText("No source"))
	require.Nil(t, noteIn(t, app, n.ID).SourceID)
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

	audio, err := factory.NoteAudio(ctx, app.DB, user.ID)
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
