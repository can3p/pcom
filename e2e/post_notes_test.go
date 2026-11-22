package e2e_test

import (
	"context"
	"net/http"
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

	// somebody else's call changes nothing
	require.Equal(t, http.StatusBadRequest, act(stranger, "note_attach", attach))
	require.Equal(t, http.StatusBadRequest, act(stranger, "note_attach_tag", map[string]string{"postId": w.post.ID, "tag": "x"}))
	require.Equal(t, both, w.belongs(t))

	require.Equal(t, http.StatusOK, act(author, "note_attach", attach))
	require.Equal(t, []string{"zqxthree loose", "zqxtwo by tag", "zqxone by hand"}, w.belongs(t))

	require.Equal(t, http.StatusOK, act(author, "note_detach", attach))
	require.Equal(t, http.StatusOK, act(author, "note_exclude", map[string]string{"noteId": w.tagged.ID, "postId": w.post.ID}))
	require.Equal(t, []string{"zqxone by hand"}, w.belongs(t))

	// the tag stays on the excluded note, and detaching the tag ends what it brought
	require.Equal(t, []string{"pond"}, testutil.Must(w.svc.Get(context.Background(), w.author, w.tagged.ID))(t).Tags)
	require.Equal(t, http.StatusOK, act(author, "note_detach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, http.StatusOK, act(author, "note_attach_tag", map[string]string{"postId": w.post.ID, "tag": "pond"}))
	require.Equal(t, []string{"zqxone by hand"}, w.belongs(t), "an excluded note stays out when its tag is attached again")
}

func TestPostNotes_ScreensShowWhatBelongsToThePost(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	author := loginAs(t, w.app, w.author)
	stranger := loginAs(t, w.app, newUser(t, w.app))

	// the panel of the editor: the post's notes lead and are flagged
	picker := author.Get("/controls/notes/picker?post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxtwo by tag", "zqxone by hand", "zqxthree loose"}, texts(picker, "[data-testid=picker-note] .np-text"))
	require.Equal(t, []string{"Detach", "Detach", "Attach"}, picker.Find("[data-testid=picker-note] [data-action='notes-picker#toggle']").Map(attrLabel))
	require.Zero(t, picker.Find("[data-testid=np-hint]").Length())

	// a new post has no notes to attach to: the controls are off
	fresh := author.Get("/controls/notes/picker?q=%23pond").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Save as a draft to attach notes."}, texts(fresh, "[data-testid=np-hint]"))
	require.Equal(t, 1, fresh.Find("button[disabled][aria-label=Attach]").Length())
	require.Equal(t, 1, fresh.Find(".np-attach-tag button[disabled]").Length())

	// the tag's confirmation is worded before anything is attached
	tagged := author.Get("/controls/notes/picker?q=%23pond&post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"#pond is attached to this post."}, texts(tagged, ".np-attach-tag .meta"))

	stranger.Get("/controls/notes/picker?post=" + w.post.ID).RequireStatus(http.StatusNotFound)

	// /controls/notes: the post filter, the posts a note is attached to, the tag's prompt
	list := author.Get("/controls/notes?post=" + w.post.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"zqxtwo by tag", "zqxone by hand"}, texts(list, "[data-testid=note] .note-body"))
	require.Equal(t, []string{"Walden"}, texts(list, "[data-testid=notes-of-post] a"))
	stranger.Get("/controls/notes?post=" + w.post.ID).RequireStatus(http.StatusNotFound)

	all := author.Get("/controls/notes?q=%23pond").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Attached to Walden ×"}, texts(all, "[data-testid=note-posts]"))
	require.Equal(t, "/controls/notes?post="+w.post.ID, all.Find("[data-testid=note-posts] a").AttrOr("href", ""))
	require.Equal(t, []string{"1 note tagged #pond is attached now, and every note you tag #pond from now on joins this post."},
		all.Find(".nt-attach button").Map(func(_ int, s *goquery.Selection) string { return s.AttrOr("data-prompt", "") }))
	require.Contains(t, all.Find("[data-testid=note] [data-action-prompt-value]").First().AttrOr("data-action-prompt-value", ""), "attached to “Walden”")
}

func TestPostNotes_InsertIntoPostOpensTheEditorWithTheText(t *testing.T) {
	t.Parallel()

	w := newPostNotesWorld(t)
	author := loginAs(t, w.app, w.author)

	doc := author.Get("/posts/" + w.post.ID + "/edit?insert=" + w.hand.ID).RequireStatus(http.StatusOK).Doc()
	require.Contains(t, doc.Find("textarea[name=body]").Text(), "zqxone by hand")
	require.Equal(t, w.hand.ID, doc.Find("[data-notes-picker-inserted]").AttrOr("data-notes-picker-inserted", ""))

	// a note of somebody else is not found
	theirs := mustNote(t, w.app, w.reader.ID, factory.WithNoteBody("zqxtheirs"))
	author.Get("/posts/" + w.post.ID + "/edit?insert=" + theirs.ID).RequireStatus(http.StatusNotFound)
	author.Get("/posts/" + w.post.ID + "/edit?insert=nope").RequireStatus(http.StatusNotFound)
}
