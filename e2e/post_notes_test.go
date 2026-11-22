package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// postNotesWorld is an author with a published post that has two notes, one
// attached by hand and one by tag, a note nobody attached, and a reader.
type postNotesWorld struct {
	app                 *e2e.App
	author, reader      *model.User
	post                *model.Post
	hand, tagged, loose *model.Note
	svc                 *notes.Service
}

func newPostNotesWorld(t *testing.T) *postNotesWorld {
	t.Helper()

	ctx := context.Background()
	app := e2e.Start(t)
	w := &postNotesWorld{app: app, svc: notes.New(repo.New(app.DB))}

	w.author = newUser(t, app)
	w.reader = newUser(t, app)
	connectUsers(t, app, w.author, w.reader)

	w.post = newPost(t, app, w.author.ID, factory.WithSubject("Walden"), factory.Published())
	w.hand = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxone by hand"), factory.CapturedAt(day(1)))
	w.tagged = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxtwo by tag"), factory.WithNoteTags("pond"), factory.CapturedAt(day(2)))
	w.loose = mustNote(t, app, w.author.ID, factory.WithNoteBody("zqxthree loose"), factory.CapturedAt(day(3)))

	require.NoError(t, w.svc.Attach(ctx, w.author, w.hand.ID, w.post.ID))
	testutil.Must(w.svc.AttachTag(ctx, w.author, w.post.ID, "pond"))(t)

	return w
}

func attrLabel(_ int, s *goquery.Selection) string { return s.AttrOr("aria-label", "") }

// belongs lists the bodies of the notes that belong to the post.
func (w *postNotesWorld) belongs(t *testing.T) []string {
	t.Helper()

	pn := testutil.Must(w.svc.ForPost(context.Background(), w.author, w.post.ID))(t)

	var out []string
	for _, n := range pn.Notes {
		out = append(out, n.Note.Body)
	}

	return out
}

func TestPostNotes_TheAuthorsPageOffersThemAndNobodyElses(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	other := mustNote(t, w.app, w.reader.ID, factory.WithNoteBody("zqxreaders own"), factory.WithNoteTags("pond"))

	page := "/posts/" + w.post.ID
	doc := loginAs(t, w.app, w.author).Get(page).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Comments (0)", "Notes (2)"}, texts(doc, "[data-controller=post-notes] [role=tab]"))

	resp := loginAs(t, w.app, w.reader).Get(page).RequireStatus(http.StatusOK)
	require.Zero(t, resp.Doc().Find("[data-controller=post-notes], [role=tab]").Length())
	require.NotContains(t, resp.Body, "post-notes")

	section := loginAs(t, w.app, w.author).Get(page + "/notes").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Attached one by one", "Tagged #pond"}, texts(section, ".post-notes-title"))
	require.Equal(t, []string{"zqxone by hand"}, texts(section, "section[aria-label='Attached one by one'] .note-body"))
	require.Equal(t, []string{"zqxtwo by tag"}, texts(section, "section[aria-label='Tagged #pond'] .note-body"))
	require.NotContains(t, section.Text(), other.Body)
	require.Equal(t, []string{"×"}, texts(section, ".post-notes-attached button"))
	require.Equal(t, []string{"#pond"}, texts(section, "ul[aria-label='Filter the notes by tag'] a"))

	narrowed := loginAs(t, w.app, w.author).Get(page + "/notes?tag=pond").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxtwo by tag"}, texts(narrowed, ".note-body"))
}

func TestPostNotes_ActionsAreTheAuthorsAlone(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	author := loginAs(t, w.app, w.author)
	stranger := loginAs(t, w.app, newUser(t, w.app))
	both := []string{"zqxtwo by tag", "zqxone by hand"}

	act := func(c *e2e.Client, name string, payload map[string]string) int {
		return c.PostJSON("/controls/action/"+name, payload).StatusCode
	}
	attach := map[string]string{"noteId": w.loose.ID, "postId": w.post.ID}

	// somebody else's calls change nothing
	require.Equal(t, http.StatusBadRequest, act(stranger, "note_attach", attach))
	require.Equal(t, http.StatusBadRequest, act(stranger, "note_attach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, http.StatusBadRequest, act(stranger, "note_detach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, both, w.belongs(t))
	require.Equal(t, []string{"pond"}, testutil.Must(w.svc.ForPost(context.Background(), w.author, w.post.ID))(t).Tags)

	// the author's reach the service
	require.Equal(t, http.StatusOK, act(author, "note_attach", attach))
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, w.belongs(t))
	require.Equal(t, http.StatusOK, act(author, "note_detach", attach))
	require.Equal(t, http.StatusOK, act(author, "note_exclude", map[string]string{"noteId": w.tagged.ID, "postId": w.post.ID}))
	require.Equal(t, []string{"zqxone by hand"}, w.belongs(t))
	require.Equal(t, http.StatusOK, act(author, "note_detach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, http.StatusOK, act(author, "note_attach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, []string{"pond"}, testutil.Must(w.svc.ForPost(context.Background(), w.author, w.post.ID))(t).Tags)
}

func TestPostNotes_ScreensShowWhatBelongsToThePost(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	author := loginAs(t, w.app, w.author)
	stranger := loginAs(t, w.app, newUser(t, w.app))
	cards := "[data-testid=picker-note] .np-text"

	// the editor's panel opens on the post's notes when it has some, and the
	// switch shows all of them; each card says whether the note belongs
	picker := author.Get("/controls/notes/picker?post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxtwo by tag", "zqxone by hand"}, texts(picker, cards))
	require.Equal(t, []string{"This post", "All notes"}, texts(picker, ".np-scope a"))
	require.Equal(t, "true", picker.Find(".np-scope a").First().AttrOr("aria-current", ""))
	require.Equal(t, []string{"Detach", "Detach"}, picker.Find("[data-action='notes-picker#toggle']").Map(attrLabel))
	require.Zero(t, picker.Find("[data-testid=np-hint]").Length())

	all := author.Get(picker.Find(".np-scope a").Eq(1).AttrOr("href", "")).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, texts(all, cards))
	require.Equal(t, []string{"Attach", "Detach", "Detach"}, all.Find("[data-action='notes-picker#toggle']").Map(attrLabel))

	// a post without notes opens on all of them
	empty := newPost(t, w.app, w.author.ID)
	require.Len(t, author.Get("/controls/notes/picker?post="+empty.ID).RequireStatus(http.StatusOK).Doc().Find("[data-testid=picker-note]").Nodes, 3)

	// a new post has no notes to attach to: the controls are off
	fresh := author.Get("/controls/notes/picker?q=%23pond").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Save as a draft to attach notes."}, texts(fresh, "[data-testid=np-hint]"))
	require.Equal(t, 1, fresh.Find("button[disabled][aria-label=Attach]").Length())
	require.Equal(t, 1, fresh.Find(".np-attach-tag button[disabled]").Length())

	tagged := author.Get("/controls/notes/picker?q=%23pond&post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"#pond is attached to this post."}, texts(tagged, ".np-attach-tag .meta"))

	stranger.Get("/controls/notes/picker?post=" + w.post.ID).RequireStatus(http.StatusNotFound)

	// /controls/notes leaves out the notes in posts unless asked for all of them
	list := author.Get("/controls/notes").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxthree loose"}, texts(list, "[data-testid=note] .note-body"))
	every := author.Get("/controls/notes?all=1").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, texts(every, "[data-testid=note] .note-body"))
	require.Equal(t, []string{"Attached to Walden ×", "Attached to Walden ×"}, texts(every, "[data-testid=note-posts]"))

	// the filter by post shows its notes whatever the list hides, with a chip that removes it
	byPost := author.Get("/controls/notes?post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxtwo by tag", "zqxone by hand"}, texts(byPost, "[data-testid=note] .note-body"))
	require.Equal(t, []string{"Notes of Walden ×"}, texts(byPost, "[data-testid=notes-of-post]"))
	require.Equal(t, "/controls/notes", byPost.Find("[data-testid=notes-of-post] .nt-x").AttrOr("href", ""))
	require.Equal(t, "/controls/notes?post="+w.post.ID, every.Find("[data-testid=note-posts] a").First().AttrOr("href", ""))
	stranger.Get("/controls/notes?post=" + w.post.ID).RequireStatus(http.StatusNotFound)

	// the delete confirmation names the post
	require.Contains(t, byPost.Find("[data-testid=note] [data-action-prompt-value]").First().AttrOr("data-action-prompt-value", ""), "attached to “Walden”")

	// the post page's own section links to the filter, and each note's Open to its edit page
	section := author.Get("/posts/" + w.post.ID + "/notes").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "/controls/notes?post="+w.post.ID, linkHref(section.Selection, "Open in Notes"))
	require.Equal(t, "/controls/notes/"+w.hand.ID+"/edit", section.Find("[data-testid=note]:contains('zqxone by hand') a:contains('Open')").AttrOr("href", ""))
	require.Zero(t, section.Find("a:contains('Insert into post')").Length())
}

func TestPostNotes_AttachPicksFromTheNewestSavedPosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := e2e.Start(t, e2e.WithEnv("NOTE_ATTACH_POSTS", "2"))
	user, c := newLoggedIn(t, app)
	mustNote(t, app, user.ID, factory.WithNoteBody("zqxany"))

	for _, subject := range []string{"Oldest", "Middle", "Newest"} {
		testutil.Must(factory.Post(ctx, app.DB, user.ID, factory.WithSubject(subject)))(t)
	}

	// an unsaved new post is a scratch pad, not a post to attach to
	c.PostForm("/controls/form/edit_post", url.Values{
		"subject": {"Scratch"}, "body": {"only a pad"}, "visibility": {string(model.PostVisibilitySecondDegree)}, "save_action": {"autosave"},
	}).RequireStatus(http.StatusOK)

	doc := c.Get("/controls/notes").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Newest (draft)", "Middle (draft)"}, texts(doc, ".note-attach-form option"))
}
