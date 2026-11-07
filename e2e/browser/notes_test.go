//go:build browser

// Everything an author does on the notes pages: the tag bar (include,
// exclude), the search box, the Inbox with its suggestions and selections, sources, renaming and merging, the edit page, quick capture and
// each delete with its confirmation.
package browser_test

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
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

// A tap on a chip includes the tag, a second tap excludes it, a third clears
// it; the search box takes #tags, -#tags and text.
func TestNotes_TagBar(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)

	mustNote(t, app, user.ID, factory.WithNoteBody("alpha note"), factory.WithNoteTags("essay"))
	mustNote(t, app, user.ID, factory.WithNoteBody("bravo note"), factory.WithNoteTags("essay", "done"))
	mustNote(t, app, user.ID, factory.WithNoteBody("charlie note"), factory.WithNoteTags("walk"))
	mustNote(t, app, user.ID, factory.WithNoteBody("delta note"))

	page := browser.Page(t, app, browser.As(user))
	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	bar := page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "Tags"})
	notes := page.GetByTestId("note")
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(4))

	require.NoError(t, bar.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#essay"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`tag=essay`)))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "alpha note")).ToBeVisible())

	require.NoError(t, bar.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#essay"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`not=essay`)))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "charlie note")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "delta note")).ToBeVisible())

	require.NoError(t, bar.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#essay"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes$`)))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(4))

	search := page.GetByRole("searchbox", playwright.PageGetByRoleOptions{Name: "Search notes"})
	require.NoError(t, search.Fill("#Essay -#done note"))
	require.NoError(t, search.Press("Enter"))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`q=`)))
	require.NoError(t, browser.Expect.Locator(notes).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(noteCard(page, "alpha note")).ToBeVisible())
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
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Not saved")).ToBeVisible())

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete"}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes$`)))
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.True(t, noteGone(t, app, note.ID))
}

// A note typed into the capture box with Ctrl+Enter lands in the list; the
// Notes section of the controls page counts the Inbox and links to the list.
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
