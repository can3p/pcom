//go:build browser

// The notes of a post: the author's Comments | Notes toggle on the post page,
// and attaching a note, attaching a tag, hiding a note and detaching from
// /controls/notes, the post editor's Notes tab and the post page.
package browser_test

import (
	"context"
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
// author's notes and only theirs; the choice is kept; a reader has no toggle.
func TestPostNotes_AuthorToggleOnThePostPage(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(postTab(page, "Comments (0)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByRole("tabpanel", playwright.PageGetByRoleOptions{Name: "Notes (2)"})).ToBeHidden())

	require.NoError(t, postTab(page, "Notes (2)").Click())

	section := page.GetByTestId("post-notes")
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(section.GetByRole("region", playwright.LocatorGetByRoleOptions{Name: "Attached one by one"}).GetByText("zqxone by hand")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(section.GetByRole("region", playwright.LocatorGetByRoleOptions{Name: "Tagged #pond"}).GetByText("zqxtwo by tag")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(section.GetByText("zqxthree loose")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(section.GetByText("zqxreaders own")).ToHaveCount(0))

	// the tag bar narrows the notes, and its active tag takes the filter off
	require.NoError(t, section.GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Filter the notes by tag"}).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#pond"}).Click())
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(1))
	require.NoError(t, section.GetByRole("list", playwright.LocatorGetByRoleOptions{Name: "Filter the notes by tag"}).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "#pond"}).Click())
	require.NoError(t, browser.Expect.Locator(section.GetByTestId("note")).ToHaveCount(2))

	// the last choice is kept for this browser
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(postTab(page, "Notes (2)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes").GetByTestId("note")).ToHaveCount(2))

	require.NoError(t, postTab(page, "Comments (0)").Click())
	require.NoError(t, browser.Expect.Locator(postTab(page, "Comments (0)")).ToHaveAttribute("aria-selected", "true"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("post-notes")).ToBeHidden())

	// a reader sees the comments as before: no toggle, nothing of the notes
	reader := browser.Page(t, w.app, browser.As(w.reader))
	_, err = reader.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(reader.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Comments"})).ToHaveCount(1))
	require.NoError(t, browser.Expect.Locator(reader.GetByRole("tab")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(reader.GetByText("zqxone by hand")).ToHaveCount(0))
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

// "Insert into post" opens the editor with the note's text at the end, saved,
// and the note counts as used.
func TestPostNotes_InsertIntoPostFromThePostPage(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))

	_, err := page.Goto("/posts/" + w.post.ID)
	require.NoError(t, err)
	require.NoError(t, postTab(page, "Notes (2)").Click())
	require.NoError(t, page.GetByTestId("post-notes").GetByTestId("note").Filter(playwright.LocatorFilterOptions{HasText: "zqxone by hand"}).
		GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Insert into post"}).Click())

	require.NoError(t, browser.Expect.Locator(page.GetByPlaceholder("Your post goes there")).ToHaveValue(w.post.Body+"\n\nzqxone by hand"))

	require.Eventually(t, func() bool {
		post, err := factory.GetPost(context.Background(), w.app.DB, w.post.ID)
		links, lerr := w.svc.PostLinks(context.Background(), w.author, w.hand.ID)

		return err == nil && lerr == nil && strings.HasSuffix(post.Body, "zqxone by hand") && links == 1
	}, 10*time.Second, 100*time.Millisecond)
}

// On /controls/notes: a note is attached to a post chosen from the picker and
// the list says so; its ×, "Notes of post" and the tag's attach (confirmed
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

	loose := noteCard(page, "zqxthree loose")
	require.NoError(t, loose.GetByText("Attach to post…").Click())
	_, err = loose.GetByLabel("Post to attach the note to").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Another draft (draft)"}})
	require.NoError(t, err)
	require.NoError(t, loose.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach", Exact: new(true)}).Click())

	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxthree loose").GetByTestId("note-posts")).ToContainText("Attached to Another draft"))
	require.Contains(t, w.postsOf(t, w.loose.ID), other.ID)

	// the delete confirmation names the post
	dialogs.reply(false, "")
	require.NoError(t, noteCard(page, "zqxthree loose").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete note"}).Click())
	require.Contains(t, dialogs.last(), "attached to “Another draft”")
	dialogs.reply(true, "")

	// the list of the post's notes
	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	_, err = page.GetByLabel("Notes of post").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Walden"}})
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Show"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("notes-of-post")).ToContainText("Walden"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(2))

	// × detaches a note from a post
	_, err = page.Goto("/controls/notes")
	require.NoError(t, err)
	require.NoError(t, noteCard(page, "zqxthree loose").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach from Another draft"}).Click())
	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxthree loose").GetByTestId("note-posts")).ToHaveCount(0))
	require.NotContains(t, w.postsOf(t, w.loose.ID), other.ID)

	// a tag's notes: the confirmation counts them now and promises the later ones
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxfour reeds"), factory.WithNoteTags("reeds"))
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxfive reeds"), factory.WithNoteTags("reeds"))

	_, err = page.Goto("/controls/notes?q=%23reeds")
	require.NoError(t, err)
	_, err = page.GetByLabel("Post", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Labels: &[]string{"Another draft (draft)"}})
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Attach all notes tagged #reeds"}).Click())

	require.NoError(t, browser.Expect.Locator(noteCard(page, "zqxfour reeds").GetByTestId("note-posts")).ToContainText("Attached to Another draft"))
	require.Equal(t, "2 notes tagged #reeds are attached now, and every note you tag #reeds from now on joins this post.", dialogs.last())
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

// In the editor's Notes tab: the post's notes lead, Attach and Detach act on a
// card, a tag of the search is attached after its confirmation, and a new
// post, which is not a draft yet, has the controls off.
func TestPostNotes_AttachFromTheEditor(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	page := browser.Page(t, w.app, browser.As(w.author))
	dialogs := watchDialogs(page)

	_, err := page.Goto("/posts/" + w.post.ID + "/edit")
	require.NoError(t, err)
	openNotes(t, page)

	require.NoError(t, browser.Expect.Locator(pickerCards(page)).ToHaveCount(3))
	require.NoError(t, browser.Expect.Locator(pickerCards(page).Nth(0)).ToContainText("zqxtwo by tag"))
	require.NoError(t, browser.Expect.Locator(pickerCards(page).Nth(1)).ToContainText("zqxone by hand"))
	require.NoError(t, browser.Expect.Locator(pickerCards(page).Nth(2)).ToContainText("zqxthree loose"))
	require.NoError(t, browser.Expect.Locator(pickerCards(page).GetByTestId("np-attached").Nth(0)).ToBeVisible())

	attach := func(card string) playwright.Locator {
		return pickerCard(page, card).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach", Exact: new(true)})
	}
	detach := func(card string) playwright.Locator {
		return pickerCard(page, card).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Detach", Exact: new(true)})
	}

	require.NoError(t, attach("zqxthree loose").Click())
	require.NoError(t, browser.Expect.Locator(detach("zqxthree loose")).ToBeVisible())
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, w.belongs(t))

	// a note that a tag brings is hidden from the post, not untagged
	require.NoError(t, detach("zqxtwo by tag").Click())
	require.NoError(t, browser.Expect.Locator(attach("zqxtwo by tag")).ToBeVisible())
	require.Equal(t, []string{"zqxthree loose", "zqxone by hand"}, w.belongs(t))

	require.NoError(t, detach("zqxthree loose").Click())
	require.NoError(t, browser.Expect.Locator(attach("zqxthree loose")).ToBeVisible())
	require.Equal(t, []string{"zqxone by hand"}, w.belongs(t))

	// a tag of the search
	require.NoError(t, w.svc.DetachTag(context.Background(), w.author, w.post.ID, "pond"))
	mustNote(t, w.app, w.author.ID, factory.WithNoteBody("zqxsix reeds"), factory.WithNoteTags("reeds"))
	searchNotes(t, page, "#reeds")
	require.NoError(t, notesPanel(page).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach all notes tagged #reeds"}).Click())
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("#reeds is attached to this post.")).ToBeVisible())
	require.Equal(t, "1 note tagged #reeds is attached now, and every note you tag #reeds from now on joins this post.", dialogs.last())
	require.Equal(t, []string{"reeds"}, w.tags(t))

	// a new post: nothing to attach to yet
	_, err = page.Goto("/write")
	require.NoError(t, err)
	openNotes(t, page)
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByText("Save as a draft to attach notes.")).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(pickerCards(page).First().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach", Exact: new(true)})).ToBeDisabled())
	searchNotes(t, page, "#reeds")
	require.NoError(t, browser.Expect.Locator(notesPanel(page).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Attach all notes tagged #reeds"})).ToBeDisabled())
}
