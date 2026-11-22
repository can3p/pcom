//go:build browser

// The notes of a post: the author's Comments | Notes toggle on the post page,
// and attaching a note, attaching a tag, hiding a note and detaching from
// /controls/notes, the post editor's Notes tab and the post page.
package browser_test

import (
	"context"
	"regexp"
	"strings"
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

// postNotesWorld is an author with a published post, "Walden", that has one
// note attached by hand and one by the tag pond, a loose note, a connected
// reader, and a note of the reader's own that carries the same tag.
type postNotesWorld struct {
	app          *e2e.App
	author       *model.User
	reader       *model.User
	post         *model.Post
	svc          *notesvc.Service
	hand, pooled *model.Note
	loose        *model.Note
}

func newPostNotesWorld(t *testing.T) *postNotesWorld {
	t.Helper()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithRealAssets())
	w := &postNotesWorld{app: app, svc: notesvc.New(repo.New(app.DB))}

	w.author = browser.NewUser(t, app)
	w.reader = browser.NewUser(t, app)

	_, _, err := factory.Connect(ctx, app.DB, w.author.ID, w.reader.ID)
	require.NoError(t, err)

	w.post = testutil.Must(factory.Post(ctx, app.DB, w.author.ID, factory.WithSubject("Walden"), factory.Published()))(t)
	w.hand = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxone by hand"))
	w.pooled = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxtwo by tag"), factory.WithNoteTags("pond"))
	w.loose = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxthree loose"))
	mustNote(t, app, w.reader.ID, factory.WithNoteBody("zqxreaders own"), factory.WithNoteTags("pond"))

	require.NoError(t, w.svc.Attach(ctx, w.author, w.hand.ID, w.post.ID))
	testutil.Must(w.svc.AttachTag(ctx, w.author, w.post.ID, "pond"))(t)

	return w
}

// belongs lists the bodies of the notes that belong to the post, newest
// capture first.
func (w *postNotesWorld) belongs(t *testing.T) []string {
	t.Helper()

	pn := testutil.Must(w.svc.ForPost(context.Background(), w.author, w.post.ID))(t)

	var out []string
	for _, n := range pn.Notes {
		out = append(out, n.Note.Body)
	}

	return out
}

func (w *postNotesWorld) tags(t *testing.T) []string {
	t.Helper()

	return testutil.Must(w.svc.ForPost(context.Background(), w.author, w.post.ID))(t).Tags
}

func postTab(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("tab", playwright.PageGetByRoleOptions{Name: name})
}

// The post page of its author has the toggle, comments first; Notes loads the
// author's notes and only theirs; the choice is kept.
func TestPostNotes_AuthorToggleOnThePostPage(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(postTab(page, "Comments (0)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.Locator("#post-pane-notes")).ToBeHidden())

	require.NoError(t, postTab(page, "Notes (2)").Click())
	require.NoError(t, browser.Expect.Locator(page.Locator("#post-pane-notes")).ToBeVisible())

	section := page.GetByTestId("post-notes")
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(section.GetByRole("region", playwright.LocatorGetByRoleOptions{Name: "Attached one by one"}).GetByText("zqxone by hand")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(section.GetByRole("region", playwright.LocatorGetByRoleOptions{Name: "Tagged #pond"}).GetByText("zqxtwo by tag")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(section.GetByText("zqxthree loose")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(section.GetByText("zqxreaders own")).ToHaveCount(0))

	// each note opens on its own page, and the section on the list of the post's notes
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note").Filter(playwright.LocatorFilterOptions{HasText: "zqxone by hand"}).
		GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Open"})).ToHaveAttribute("href", "/controls/notes/"+w.hand.ID+"/edit"))
	require.NoError(t, browser.Expect.Locator(section.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Open in Notes"})).ToHaveAttribute("href", "/controls/notes?post="+w.post.ID))

	// the tag bar narrows the notes, and its active tag takes the filter off
	bar := section.GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Filter the notes by tag"}).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#pond"})
	require.NoError(t, bar.Click())
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(1))
	require.NoError(t, bar.Click())
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(2))

	// the tab is in the address, which a reload keeps
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+w.post.ID+`#notes$`)))
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (2)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(2))

	require.NoError(t, postTab(page, "Comments (0)").Click())
	require.NoError(t, browser.Expect.Locator(postTab(page, "Comments (0)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes")).ToBeHidden())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+w.post.ID+`#comments$`)))
}

// A link to /posts/<id>#notes opens on the notes, loaded on the first paint.
func TestPostNotes_AddressOpensTheNotesTab(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + w.post.ID + "#notes")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (2)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(2))

	_, err = page.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(postTab(page, "Comments (0)")).ToHaveAttribute("aria-selected", "true"))
}

// A draft has no comments, so its page opens on the notes; Detach takes a note
// attached by hand off, hides one a tag brings (it keeps the tag), and the
// tag's own Detach ends what it attached.
func TestPostNotes_DetachFromThePostPage(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	ctx := context.Background()

	draft := testutil.Must(factory.Post(ctx, w.app.DB, w.author.ID, factory.WithSubject("A draft")))(t)
	require.NoError(t, w.svc.Attach(ctx, w.author, w.loose.ID, draft.ID))

	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + draft.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (1)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByText("zqxthree loose")).ToBeVisible())

	_, err = page.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)
	require.NoError(t, postTab(page, "Notes (2)").Click())

	section := page.GetByTestId("post-notes")
	card := func(text string) playwright.Locator {
		return section.GetByTestId("note").Filter(playwright.LocatorFilterOptions{HasText: text})
	}

	require.NoError(t, card("zqxone by hand").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach"}).Click())
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (1)")).ToBeVisible())
	require.Equal(t, []string{"zqxtwo by tag"}, w.belongs(t))

	require.NoError(t, card("zqxtwo by tag").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach"}).Click())
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (0)")).ToBeVisible())
	require.Empty(t, w.belongs(t))
	require.Equal(t, []string{"pond"}, noteIn(t, w.app, w.pooled.ID).Tags, "hidden from the post, still tagged")
	require.NoError(t, browser.Expect.Locator(section.GetByText("No notes are attached to this post yet.")).ToBeVisible())

	require.NoError(t, section.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach tag #pond"}).Click())
	require.NoError(t, browser.Expect.Locator(section.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach tag #pond"})).ToHaveCount(0))
	require.Empty(t, w.tags(t))
}

// On /controls/notes: a note is attached to a post chosen from the picker, and
// the list says so; its ×, the post's own list and the tag's attach (confirmed
// first) all work.
func TestPostNotes_AttachFromTheNotesList(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	ctx := context.Background()
	other := testutil.Must(factory.Post(ctx, w.app.DB, w.author.ID, factory.WithSubject("Another draft")))(t)

	page := browser.Page(t, w.app, browser.As(w.author))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	// the notes in posts stay out of the list until it is asked for all of them
	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxone by hand")).ToHaveCount(0))

	loose := noteCard(page, "zqxthree loose")
	require.NoError(t, loose.GetByText("Attach to post…").Click())
	_, err = loose.GetByLabel("Post to attach the note to").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Another draft (draft)"}})
	require.NoError(t, err)
	require.NoError(t, loose.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach", Exact: new(true)}).Click())

	// it belongs to a post now, so the list no longer shows it
	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxthree loose")).ToHaveCount(0))
	require.Contains(t, w.postsOf(t, w.loose.ID), other.ID)

	// all the notes say which posts they belong to; the link filters by the post
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "All notes"}).Click())
	require.NoError(t, noteCard(page, "zqxone by hand").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Walden"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("notes-of-post")).ToContainText("Notes of Walden"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(2))

	// the chip's × takes the filter off
	require.NoError(t, page.GetByTestId("notes-of-post").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Remove the post filter"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("notes-of-post")).ToHaveCount(0))

	// × detaches a note from a post
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "All notes"}).Click())
	require.NoError(t, noteCard(page, "zqxthree loose").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach from Another draft"}).Click())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxthree loose").GetByTestId("note-posts")).ToHaveCount(0))
	require.NotContains(t, w.postsOf(t, w.loose.ID), other.ID)

	// a tag's notes: confirmed first, and the notes tagged later join the post
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxfour reeds"), factory.WithNoteTags("reeds"))
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxfive reeds"), factory.WithNoteTags("reeds"))

	_, err = page.Goto("/controls/notes?q=%23reeds")
	require.NoError(t, err)
	_, err = page.GetByLabel("Post", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Another draft (draft)"}})
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Attach all notes tagged #reeds"}).Click())

	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxfour reeds")).ToHaveCount(0))
	require.NotEmpty(t, dialogs.last())
	require.Equal(t, []string{"reeds"}, testutil.Must(w.svc.ForPost(ctx, w.author, other.ID))(t).Tags)
	require.Len(t, testutil.Must(w.svc.ForPost(ctx, w.author, other.ID))(t).Notes, 2)
}

func (w *postNotesWorld) postsOf(t *testing.T, noteID string) []string {
	t.Helper()

	var ids []string
	for _, p := range testutil.Must(w.svc.PostsOf(context.Background(), w.author, noteID))(t) {
		ids = append(ids, p.ID)
	}

	return ids
}

func attachButton(page playwright.Page, card, name string) playwright.Locator {
	return pickerCard(page, card).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: name, Exact: new(true)})
}

// In the editor's Notes tab: it opens on the post's notes when it has some,
// the switch shows all of them with the post's flagged, Attach and Detach act
// on a card, a tag of the search is attached after its confirmation, and an
// inserted note is attached.
func TestPostNotes_AttachFromTheEditor(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))
	watchDialogs(page)

	_, err := page.Goto("/posts/" + w.post.ID + "/edit")
	require.NoError(t, err)
	openNotes(t, page)

	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxthree loose")).ToHaveCount(0))

	// what is shown is said, and the selected view of the switch is marked
	caption := notesPanel(page).GetByTestId("np-caption")
	require.NoError(t, browser.Expect.Locator(caption).ToHaveText("Showing the notes of this post"))
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "This post"})).ToHaveAttribute("aria-current", "true"))
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"})).Not().ToHaveAttribute("aria-current", "true"))
	// an attached note's button is the pressed Detach
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxone by hand").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach", Exact: new(true), Pressed: new(true)})).ToBeVisible())

	require.NoError(t, notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"}).Click())
	require.NoError(t, browser.Expect.Locator(caption).ToHaveText("Showing all notes"))
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"})).ToHaveAttribute("aria-current", "true"))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxthree loose").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach", Exact: new(true), Pressed: new(false)})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(3))
	require.NoError(t, browser.Expect.Locator(pickerCards(page).GetByTestId("np-attached").Locator("visible=true")).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxthree loose").GetByTestId("np-attached")).ToBeHidden())

	require.NoError(t, attachButton(page, "zqxthree loose", "Attach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Detach")).ToBeVisible())
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, w.belongs(t))

	// a note that a tag brings is hidden from the post, not untagged
	require.NoError(t, attachButton(page, "zqxtwo by tag", "Detach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxtwo by tag", "Attach")).ToBeVisible())
	require.NoError(t, attachButton(page, "zqxthree loose", "Detach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Attach")).ToBeVisible())
	require.Equal(t, []string{"zqxone by hand"}, w.belongs(t))

	// a tag of the search
	require.NoError(t, w.svc.DetachTag(context.Background(), w.author, w.post.ID, "pond"))
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxsix reeds"), factory.WithNoteTags("reeds"))
	searchNotes(t, page, "#reeds")
	require.NoError(t, notesPanel(page).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach all notes tagged #reeds"}).Click())
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("#reeds is attached to this post.")).ToBeVisible())
	require.Equal(t, []string{"reeds"}, w.tags(t))

	// inserting a note attaches it
	require.NoError(t, notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "This post"}).Click())
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "This post"})).ToHaveAttribute("aria-current", "true"))
	searchNotes(t, page, "zqxthree")
	// searching on This post searches the post's notes only, and the switch stays
	all := notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"})
	require.NoError(t, browser.Expect.Locator(all).ToHaveAttribute("href", regexp.MustCompile(`q=zqxthree`)))
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "This post"})).ToHaveAttribute("aria-current", "true"))
	require.NoError(t, all.Click())
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxthree loose")).ToBeVisible())

	// and on All notes it searches them all
	searchNotes(t, page, "zqxsix")
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxsix reeds")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"})).ToHaveAttribute("aria-current", "true"))
	searchNotes(t, page, "zqxthree")
	require.NoError(t, browser.Expect.Locator(pickerCard(page, "zqxthree loose")).ToBeVisible())
	require.NoError(t, pickerCard(page, "zqxthree loose").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Insert", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Detach")).ToBeVisible())
	require.Eventually(t, func() bool {
		return strings.Contains(strings.Join(w.belongs(t), "|"), "zqxthree loose")
	}, 10*time.Second, 100*time.Millisecond)
}

// A draft, and a new post once it is saved as a draft: the panel offers Attach
// without reloading the page.
func TestPostNotes_AttachFromTheEditorOfADraft(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/write")
	require.NoError(t, err)
	openNotes(t, page)

	// nothing to attach to yet
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("Save as a draft to attach notes.")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Attach")).ToBeDisabled())

	require.NoError(t, page.GetByPlaceholder("Subject").Fill("A new draft"))
	require.NoError(t, page.GetByPlaceholder("Your post goes there").Fill("Some words"))
	savedPad(t, w.app, w.author.ID, "Some words")
	require.NoError(t, writeButton(page, "Save as Draft").Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(b2EditURLRe))

	// the panel learns the post: no reload
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Attach")).ToBeEnabled())
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("Save as a draft to attach notes.")).ToHaveCount(0))

	require.NoError(t, attachButton(page, "zqxthree loose", "Attach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Detach")).ToBeVisible())

	drafts := testutil.Must(w.svc.AttachablePosts(context.Background(), w.author))(t)
	require.Equal(t, "A new draft", *drafts[0].Subject)
	require.Equal(t, []string{"zqxthree loose"}, bodiesOfPost(t, w, drafts[0].ID))

	require.NoError(t, attachButton(page, "zqxthree loose", "Detach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Attach")).ToBeVisible())
	require.Empty(t, bodiesOfPost(t, w, drafts[0].ID))
}

func bodiesOfPost(t *testing.T, w *postNotesWorld, postID string) []string {
	t.Helper()

	var out []string
	for _, n := range testutil.Must(w.svc.ForPost(context.Background(), w.author, postID))(t).Notes {
		out = append(out, n.Note.Body)
	}

	return out
}

// Attaching and detaching in the editor, saving, and going to the post's notes
// shows the notes in the pane, and nothing else.
func TestPostNotes_NotesPaneAfterSavingFromTheEditor(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + w.post.ID + "#notes")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(2))

	// into the editor by its link, as a boosted visit
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Edit", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+w.post.ID+`/edit$`)))
	openNotes(t, page)
	require.NoError(t, notesPanel(page).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "All notes"}).Click())
	require.NoError(t, attachButton(page, "zqxthree loose", "Attach").Click())
	require.NoError(t, attachButton(page, "zqxthree loose", "Detach").Click())
	require.NoError(t, attachButton(page, "zqxthree loose", "Attach").Click())
	require.NoError(t, browser.Expect.Locator(attachButton(page, "zqxthree loose", "Detach")).ToBeVisible())
	require.NoError(t, writeButton(page, "Save Post").Click())

	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/posts/`+w.post.ID+`$`)))
	require.NoError(t, postTab(page, "Notes (3)").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(3))
	require.NoError(t, browser.Expect.Locator(page.Locator("#post-pane-notes").GetByRole("navigation")).ToHaveCount(0))

	// back to the editor and forward again: the tab is still the address's
	_, err = page.GoBack()
	require.NoError(t, err)
	_, err = page.GoForward()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(3))
	require.NoError(t, browser.Expect.Locator(page.Locator("#post-pane-notes").GetByRole("navigation")).ToHaveCount(0))
}

// A post page that a boosted link opens, as htmx swaps it in, still loads its
// notes: with #notes in the link, and for a draft without it.
func TestPostNotes_OpenedByABoostedLinkShowsItsNotes(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	ctx := context.Background()
	draft := testutil.Must(factory.Post(ctx, w.app.DB, w.author.ID, factory.WithSubject("Drafty")))(t)
	require.NoError(t, w.svc.Attach(ctx, w.author, w.loose.ID, draft.ID))

	page := browser.Page(t, w.app, browser.As(w.author))

	for href, want := range map[string]int{"/posts/" + w.post.ID + "#notes": 2, "/posts/" + draft.ID: 1} {
		_, err := page.Goto("/feed")
		require.NoError(t, err)

		// a link added to the page, which htmx boosts like the ones it came with
		_, err = page.Evaluate(`href => {
			const a = document.createElement("a")
			a.href = href
			a.textContent = "boosted link"
			document.body.append(a)
			htmx.process(a)
		}`, href)
		require.NoError(t, err)
		require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "boosted link"}).Click())

		require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(want), href)
		require.NoError(t, browser.Expect.Locator(page.Locator("#post-pane-notes").GetByRole("navigation")).ToHaveCount(0), href)
	}
}
