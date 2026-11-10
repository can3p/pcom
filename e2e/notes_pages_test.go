package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

const (
	sttOn  = "on"
	sttOff = "off"
)

// notesPagesApp starts the app with transcription on (against a backend nobody
// answers at) or off.
func notesPagesApp(t *testing.T, stt string, opts ...e2e.Option) *e2e.App {
	t.Helper()

	if stt == sttOn {
		opts = append(opts, e2e.WithEnv("STT_PROVIDER", "openai-compat"),
			e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", "http://127.0.0.1:9/stt"), e2e.WithEnv("STT_OPENAI_COMPAT_KEY", "unused"))
	}

	return e2e.Start(t, opts...)
}

// day is noon UTC of the day-th day of October 2026.
func day(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }

func mustNote(t *testing.T, app *e2e.App, userID string, opts ...factory.NoteOpt) *model.Note {
	t.Helper()

	return testutil.Must(factory.Note(context.Background(), app.DB, userID, opts...))(t)
}

// texts is the text of each element of sel, trimmed and with runs of
// whitespace collapsed.
func texts(doc interface {
	Find(string) *goquery.Selection
}, sel string) []string {
	out := []string{}

	doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
		out = append(out, strings.Join(strings.Fields(s.Text()), " "))
	})

	return out
}

// hrefs is the href of each element of sel.
func hrefs(doc interface {
	Find(string) *goquery.Selection
}, sel string) []string {
	out := []string{}

	doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
		out = append(out, s.AttrOr("href", ""))
	})

	return out
}

// sourceOf is the source of a note.
func sourceOf(t *testing.T, app *e2e.App, n *model.Note) *model.NoteSource {
	t.Helper()

	return testutil.Must(repo.New(app.DB).NoteSourceByID(context.Background(), *n.SourceID))(t)
}

// linkTo is the href of the link with the text, "" without one.
func linkTo(doc *goquery.Document, sel, text string) (string, bool) {
	var (
		href  string
		found bool
	)

	doc.Find(sel).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if strings.HasPrefix(strings.Join(strings.Fields(s.Text()), " "), text) {
			href, _ = s.Attr("href")
			found = true
		}

		return !found
	})

	return href, found
}

func TestNotesPages_ListFilters(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	other := newUser(t, app)
	src := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	mustNote(t, app, user.ID, factory.WithNoteBody("alpha apple"), factory.WithNoteTags("essay"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteBody("bravo banana"), factory.WithNoteTags("essay", "walk"), factory.CapturedAt(day(3)))
	mustNote(t, app, user.ID, factory.WithNoteBody("charlie Cherry"), factory.CapturedAt(day(5)))
	used := mustNote(t, app, user.ID, factory.WithNoteBody("delta date"), factory.WithNoteTags("walk"),
		factory.FromSource(src.ID), factory.CapturedAt(day(7)))
	mustNote(t, app, user.ID, factory.WithNoteBody("echo elder"), factory.WithNoteTags("secret"), factory.CapturedAt(day(9)))
	mustNote(t, app, other.ID, factory.WithNoteBody("foreign fig"), factory.WithNoteTags("essay"), factory.CapturedAt(day(10)))
	post := testutil.Must(factory.Post(ctx, app.DB, user.ID))(t)
	testutil.Must(factory.NotePostLink(ctx, app.DB, used.ID, post.ID))(t)

	list := func(query string) []string {
		return texts(c.Get("/controls/notes"+query).RequireStatus(http.StatusOK).Doc(), "[data-testid=note] .note-body")
	}

	// newest first; another user's note never in
	require.Equal(t, []string{"echo elder", "delta date", "charlie Cherry", "bravo banana", "alpha apple"}, list(""))

	tests := []struct {
		name, query string
		want        []string
	}{
		{"tag", "?q=%23essay", []string{"bravo banana", "alpha apple"}},
		{"two tags", "?q=%23essay+%23walk", []string{"bravo banana"}},
		{"not", "?q=-%23essay", []string{"echo elder", "delta date", "charlie Cherry"}},
		{"the old tag parameters are no filter", "?tag=essay&not=walk", []string{"echo elder", "delta date", "charlie Cherry", "bravo banana", "alpha apple"}},
		{"search box tag", "?q=%23Secret", []string{"echo elder"}},
		{"search box tags and text", "?q=%23essay+-%23walk+apple", []string{"alpha apple"}},
		{"search box tag left out", "?q=-%23walk+-%23secret", []string{"charlie Cherry", "alpha apple"}},
		{"inbox and unused are no filters", "?inbox=1&unused=1", []string{"echo elder", "delta date", "charlie Cherry", "bravo banana", "alpha apple"}},
		{"source", "?source=" + src.ID, []string{"delta date"}},
		{"search", "?q=cherry", []string{"charlie Cherry"}},
		{"search is trimmed", "?q=+banana+", []string{"bravo banana"}},
		{"nothing", "?q=zzz", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, list(tt.query))
		})
	}

	doc := c.Get("/controls/notes?q=zzz").Doc()
	require.Equal(t, []string{"No notes match."}, texts(doc, ".note-empty"))
	require.Equal(t, []string{"No notes yet."}, texts(loginAs(t, app, newUser(t, app)).Get("/controls/notes").Doc(), ".note-empty"))

	// the item says where it comes from and that a post has it
	doc = c.Get("/controls/notes?source=" + src.ID).Doc()
	meta := texts(doc, "[data-testid=note] .meta")[0]
	require.Contains(t, meta, src.Title)
	require.Contains(t, meta, "used in a post")
	require.Equal(t, "/controls/notes?source="+src.ID, doc.Find("[data-testid=note] .meta a").AttrOr("href", ""))
	require.Equal(t, []string{src.Title}, texts(doc, "[data-testid=note-source]"))
	require.NotContains(t, texts(c.Get("/controls/notes?q=charlie").Doc(), "[data-testid=note] .meta")[0], "used in a post")
}

func TestNotesPages_SearchViewsAndSources(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	src := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	mustNote(t, app, user.ID, factory.WithNoteTags("essay"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteTags("essay", "walk"), factory.CapturedAt(day(3)))
	mustNote(t, app, user.ID, factory.WithNoteTags("secret", "walk"), factory.FromSource(src.ID), factory.CapturedAt(day(5)))

	// the search's parts are chips whose x drops the part, the other filters kept
	doc := c.Get("/controls/notes?source=" + src.ID + "&q=%23essay+-%23walk+river").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "#essay -#walk river", doc.Find("form.nt-search input[name=q]").AttrOr("value", ""))
	require.Equal(t, []string{"#essay ×", "not #walk ×", "“river” ×"}, texts(doc, "[data-testid=note-filters] li"))
	require.Equal(t, []string{
		"/controls/notes?q=-%23walk+river&source=" + src.ID,
		"/controls/notes?q=%23essay+river&source=" + src.ID,
		"/controls/notes?q=%23essay+-%23walk&source=" + src.ID,
	}, hrefs(doc, "[data-testid=note-filters] a.nt-x"))
	require.Equal(t, "Remove not #walk", doc.Find("[data-testid=note-filters] a.nt-x").Eq(1).AttrOr("aria-label", ""))

	// the tags the search doesn't hold, most used first, one tap away
	require.Equal(t, []string{"#secret"}, texts(doc, "ul[aria-label='Your tags'] a"))
	require.Equal(t, []string{"/controls/notes?q=%23essay+%23secret+-%23walk+river&source=" + src.ID}, hrefs(doc, "ul[aria-label='Your tags'] a"))

	doc = c.Get("/controls/notes").Doc()
	require.Zero(t, doc.Find("[data-testid=note-filters]").Length())
	require.Equal(t, []string{"#essay", "#walk", "#secret"}, texts(doc, "ul[aria-label='Your tags'] a"))

	// a tag on a note adds it to the search on show
	doc = c.Get("/controls/notes?q=-%23secret").Doc()
	require.Equal(t, []string{"/controls/notes?q=%23essay+-%23secret", "/controls/notes?q=%23walk+-%23secret"},
		hrefs(doc.Find("[data-testid=note]").First(), ".nt-tags a"))

	// no Inbox or Unused views
	doc = c.Get("/controls/notes?q=%23essay").Doc()
	require.Zero(t, doc.Find("ul[aria-label=Views]").Length())
	require.Zero(t, doc.Find("[aria-current]").Length())

	// sources: a count, a link that selects it and, selected, one that clears it
	doc = c.Get("/controls/notes?q=%23walk").Doc()
	href, ok := linkTo(doc, "ul[aria-label=Sources] a", src.Title+" 1")
	require.True(t, ok)
	require.Equal(t, "/controls/notes?q=%23walk&source="+src.ID, href)

	doc = c.Get("/controls/notes?q=%23walk&source=" + src.ID).Doc()
	href, ok = linkTo(doc, "ul[aria-label=Sources] a[aria-current]", src.Title)
	require.True(t, ok)
	require.Equal(t, "/controls/notes?q=%23walk", href)
	require.Equal(t, src.ID, doc.Find("form.nt-search input[name=source]").AttrOr("value", ""))

	// a source without notes is no view, but the capture box offers it
	empty := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Unread book")))(t)
	doc = c.Get("/controls/notes").Doc()
	_, ok = linkTo(doc, "ul[aria-label=Sources] a", "Unread book")
	require.False(t, ok)
	require.Equal(t, []string{"No source", src.Title, "Unread book"}, texts(doc, "form.note-capture select[name=source_id] option"))
	require.Equal(t, empty.ID, doc.Find("form.note-capture select[name=source_id] option").Eq(2).AttrOr("value", ""))
}

func TestNotesPages_CursorPages(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff, e2e.WithEnv("PAGE_SIZE", "2"))
	user, c := newLoggedIn(t, app)

	for i, body := range []string{"one", "two", "three"} {
		mustNote(t, app, user.ID, factory.WithNoteBody(body), factory.WithNoteTags("x"), factory.CapturedAt(day(i+1)))
	}

	mustNote(t, app, user.ID, factory.WithNoteBody("untagged"), factory.CapturedAt(day(9)))

	doc := c.Get("/controls/notes?q=%23x").Doc()
	require.Equal(t, []string{"three", "two"}, texts(doc, "[data-testid=note] .note-body"))

	more := doc.Find(".load-more a").AttrOr("href", "")
	u, err := url.Parse(more)
	require.NoError(t, err)
	require.Equal(t, "/controls/notes", u.Path)
	require.Equal(t, "#x", u.Query().Get("q"), "the next page keeps the filter")
	require.NotEmpty(t, u.Query().Get("cursor"))

	doc = c.Get(more).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"one"}, texts(doc, "[data-testid=note] .note-body"))
	require.Zero(t, doc.Find(".load-more").Length())

	// the last page of the whole list is one short too
	doc = c.Get("/controls/notes").Doc()
	require.Equal(t, []string{"untagged", "three"}, texts(doc, "[data-testid=note] .note-body"))
}

// Once a recording on the list is transcribed, its poll answers with the
// whole list item, its tags included.
func TestNotesPages_StatusPollOnTheListAnswersTheItem(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOn)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)

	audio := testutil.Must(factory.NoteRecording(ctx, app.DB, user.ID))(t)
	n := mustNote(t, app, user.ID, factory.WithNoteBody("an essay draft"), factory.WithNoteTags("essay"), factory.WithRecording(audio.ID),
		factory.WithNoteTranscription(model.NoteTranscriptionDone))

	doc := c.Get("/controls/notes/" + n.ID + "/status?view=list").RequireStatus(http.StatusOK).Doc()
	item := doc.Find("article#note-" + n.ID)
	require.Equal(t, 1, item.Length())
	require.Equal(t, []string{"an essay draft"}, texts(item, ".note-body"))
	require.Equal(t, []string{"#essay"}, texts(item, ".nt-tags li"))
}

func TestNotesPages_ManageScreen(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)

	doc := c.Get("/controls/notes/manage").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"No tags yet."}, texts(doc, "#note-tags-title ~ ul li"))
	require.Equal(t, []string{"No sources yet."}, texts(doc, "ul[aria-label=Sources] li"))
	require.Equal(t, "/controls/form/note_source_add", doc.Find("form#note-source-add").AttrOr("hx-post", ""))

	one := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Walden")))(t)
	two := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Cape Cod")))(t)
	mustNote(t, app, user.ID, factory.WithNoteTags("essay", "walk"), factory.FromSource(one.ID))
	mustNote(t, app, user.ID, factory.WithNoteTags("essay"), factory.FromSource(one.ID))
	mustNote(t, app, user.ID, factory.WithNoteTags("secret"))

	doc = c.Get("/controls/notes/manage").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "Tags and sources", doc.Find("h1").Text())
	require.Equal(t, []string{
		"#essay 2 notes rename",
		"#secret 1 note rename",
		"#walk 1 note rename",
	}, texts(doc, "[data-testid=tag-row]"))
	require.Equal(t, "Rename #essay to (a name that exists merges the two tags)",
		doc.Find("[data-testid=tag-row] button[aria-label='Rename #essay']").AttrOr("data-action-prompt-value", ""))

	// every source, by title, with its count and an Edit link that swaps in its form
	require.Equal(t, []string{"Cape Cod 0 notes", "Walden 2 notes"}, texts(doc, "[data-testid=source-row]"))
	edit := doc.Find("[data-testid=source-row] a[aria-label='Edit Walden']")
	require.Equal(t, "/controls/notes/sources/"+one.ID+"/edit", edit.AttrOr("hx-get", ""))
	require.Equal(t, "#note-source-"+one.ID, edit.AttrOr("hx-target", ""))
	require.Zero(t, doc.Find("form.nt-merge").Length(), "renaming into another title merges; there is no merge form")

	form := c.Get("/controls/notes/sources/" + two.ID + "/edit").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "Cape Cod", form.Find("input[name=title]").AttrOr("value", ""))
	require.Equal(t, "/controls/form/note_source_edit/"+two.ID, form.Find("form").AttrOr("hx-post", ""))
	require.Equal(t, "/controls/notes/sources/"+two.ID, form.Find("a:contains('Cancel')").AttrOr("hx-get", ""))

	row := c.Get("/controls/notes/sources/" + two.ID).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Cape Cod 0 notes"}, texts(row, "[data-testid=source-row]"))
}

// The Add source form finds or makes a source by title; a source's edit form
// renames it, or merges it into the source whose title it takes. A bad title
// is an error under the field and changes nothing.
func TestNotesPages_SourceForms(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	store := repo.New(app.DB)

	count := func() int { return len(testutil.Must(store.NoteSourceCounts(ctx, user.ID))(t)) }

	resp := c.PostForm("/controls/form/note_source_add", url.Values{"title": {"  Walden   Pond "}}).RequireStatus(http.StatusOK)
	require.Equal(t, "true", resp.Header.Get("HX-Refresh"))
	walden := testutil.Must(store.NoteSourceByMatchKey(ctx, user.ID, model.NoteSourceMatchKey("walden pond")))(t)
	require.NotNil(t, walden)
	require.Equal(t, "Walden Pond", walden.Title)

	c.PostForm("/controls/form/note_source_add", url.Values{"title": {"WALDEN pond"}}).RequireStatus(http.StatusOK)
	require.Equal(t, 1, count(), "the same title is found, not added again")

	for name, title := range map[string]string{"empty": "  ", "too long": strings.Repeat("x", notes.DefaultSourceTitleMaxLength+1)} {
		doc := c.PostForm("/controls/form/note_source_add", url.Values{"title": {title}}).RequireStatus(http.StatusOK).Doc()
		require.Len(t, texts(doc, "form#note-source-add .field .field-error"), 1, name)
		require.Equal(t, "true", doc.Find("input[name=title]").AttrOr("aria-invalid", ""), name)
		require.Equal(t, 1, count(), name)
	}

	// rename: the row comes back saved
	doc := c.PostForm("/controls/form/note_source_edit/"+walden.ID, url.Values{"title": {"Walden"}}).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"Walden 0 notes Saved"}, texts(doc, "[data-testid=source-row]"))
	require.Equal(t, "Walden", sourceOf(t, app, &model.Note{SourceID: &walden.ID}).Title)

	doc = c.PostForm("/controls/form/note_source_edit/"+walden.ID, url.Values{"title": {""}}).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"A source needs a title."}, texts(doc, ".field-error"))
	require.Equal(t, "Walden", sourceOf(t, app, &model.Note{SourceID: &walden.ID}).Title)

	// rename into another source's title: its notes move there and it goes
	cape := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceTitle("Cape Cod")))(t)
	n := mustNote(t, app, user.ID, factory.FromSource(cape.ID))

	resp = c.PostForm("/controls/form/note_source_edit/"+cape.ID, url.Values{"title": {"walden"}}).RequireStatus(http.StatusOK)
	require.Equal(t, "true", resp.Header.Get("HX-Refresh"))
	require.Equal(t, walden.ID, *testutil.Must(store.NoteByID(ctx, n.ID))(t).SourceID)
	require.Equal(t, 1, count())
}

// richNote is a note with a recording in the given state, two photos, a
// source and a post that has its text.
func richNote(t *testing.T, app *e2e.App, userID string, status model.NoteTranscriptionStatus, body string) *model.Note {
	t.Helper()

	ctx := context.Background()
	src := testutil.Must(factory.NoteSource(ctx, app.DB, userID))(t)
	audio := testutil.Must(factory.NoteRecording(ctx, app.DB, userID))(t)
	n := mustNote(t, app, userID, factory.WithNoteBody(body), factory.WithNoteTags("essay"), factory.WithNoteTextTags("essay", "inline"),
		factory.WithRecording(audio.ID), factory.WithNoteTranscription(status), factory.FromSource(src.ID))

	for i := range 2 {
		up := testutil.Must(factory.MediaUpload(ctx, app.DB, userID))(t)
		testutil.Must(factory.NoteImage(ctx, app.DB, n.ID, up.ID, i))(t)
	}

	post := testutil.Must(factory.Post(ctx, app.DB, userID))(t)
	testutil.Must(factory.NotePostLink(ctx, app.DB, n.ID, post.ID))(t)

	return n
}

const (
	fullPrompt   = "Delete this note, its recording and 2 photos? This can't be undone. The text already inserted into posts stays there."
	retranscribe = "Replace the edited text with a new transcript?"
)

func TestNotesPages_EditPage(t *testing.T) {
	t.Parallel()

	on := notesPagesApp(t, sttOn)
	off := notesPagesApp(t, sttOff)

	tests := []struct {
		name      string
		app       *e2e.App
		status    model.NoteTranscriptionStatus
		body      string
		pending   bool
		wantAgain bool
	}{
		{"done with transcription on", on, model.NoteTranscriptionDone, "spoken words", false, true},
		{"done without text", on, model.NoteTranscriptionDone, "", false, true},
		{"pending", on, model.NoteTranscriptionPending, "", true, false},
		{"done with transcription off", off, model.NoteTranscriptionDone, "spoken words", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user, c := newLoggedIn(t, tt.app)
			n := richNote(t, tt.app, user.ID, tt.status, tt.body)

			doc := c.Get("/controls/notes/" + n.ID + "/edit").RequireStatus(http.StatusOK).Doc()
			require.Equal(t, "Edit note", doc.Find("h1.ed-title").Text())
			require.Equal(t, tt.body, doc.Find("textarea[name=body]").Text())
			require.Equal(t, fullPrompt, doc.Find("button.ed-delete").AttrOr("data-action-prompt-value", ""))
			require.Equal(t, n.ID, doc.Find("button.ed-delete").AttrOr("data-note-id", ""))

			// hand-set tags fill the field; the chips add the tag of the text
			require.Equal(t, "essay", doc.Find("#note-tags").AttrOr("value", ""))
			require.Equal(t, []string{"#essay", "#inline (written in the text)"}, texts(doc, "#note-tag-chips li"))

			// the Source choice is part of the note's form, the note's source chosen
			source := doc.Find("form.note-form select[name=source_id]")
			require.Equal(t, []string{"No source", sourceOf(t, tt.app, n).Title}, texts(source, "option"))
			require.Equal(t, *n.SourceID, source.Find("option[selected]").AttrOr("value", ""))
			require.Zero(t, doc.Find("form.note-source, input[name=locator], input[name=author]").Length(), "no separate source form")
			require.Zero(t, doc.Find("input[type=file][accept='audio/*']").Length(), "a note with a recording gets none added")

			if tt.pending {
				status := doc.Find("[data-testid=note-status]")
				require.Contains(t, status.Text(), "Transcribing…")
				require.Equal(t, "/controls/notes/"+n.ID+"/status?view=edit", status.AttrOr("hx-get", ""))
			} else {
				require.Empty(t, strings.TrimSpace(doc.Find("[data-testid=note-status]").Text()))
			}

			again := doc.Find("#note-again-slot [data-testid=note-again]")
			if !tt.wantAgain {
				require.Zero(t, again.Length())
				return
			}

			// the edit page always confirms: the box may hold what was typed since it opened
			require.Equal(t, retranscribe, again.AttrOr("hx-confirm", ""))
			require.Equal(t, n.ID, again.Find("input[name=note_id]").AttrOr("value", ""))
			require.Equal(t, []string{"English", "Russian", "Dutch", "German", "French", "Spanish", "Italian", "Portuguese", "Ukrainian", "Polish"},
				texts(again, "select option"))
		})
	}

	t.Run("an unused note without media has the short prompt and no recording status", func(t *testing.T) {
		t.Parallel()

		user, c := newLoggedIn(t, off)
		n := mustNote(t, off, user.ID, factory.WithNoteBody("plain"))

		doc := c.Get("/controls/notes/" + n.ID + "/edit").RequireStatus(http.StatusOK).Doc()
		require.Equal(t, "Delete this note? This can't be undone.", doc.Find("button.ed-delete").AttrOr("data-action-prompt-value", ""))
		require.Zero(t, doc.Find("[data-testid=note-status]").Length())
		require.Equal(t, []string{"No source"}, texts(doc, "select[name=source_id] option"))
		require.Equal(t, n.ID, doc.Find("input[type=file][accept='audio/*']").AttrOr("data-note-id", ""), "Add recording attaches to this note")
	})

	t.Run("another user's note is not found", func(t *testing.T) {
		t.Parallel()

		user := newUser(t, off)
		n := mustNote(t, off, user.ID)

		loginAs(t, off, newUser(t, off)).Get("/controls/notes/" + n.ID + "/edit").RequireStatus(http.StatusNotFound)
	})
}

func TestNotesPages_ListItemOfARichNote(t *testing.T) {
	t.Parallel()

	on := notesPagesApp(t, sttOn)
	off := notesPagesApp(t, sttOff)

	tests := []struct {
		name        string
		app         *e2e.App
		status      model.NoteTranscriptionStatus
		body        string
		wantAgain   bool
		wantConfirm string
	}{
		{"text to lose", on, model.NoteTranscriptionDone, "spoken words", true, retranscribe},
		{"no text to lose", on, model.NoteTranscriptionDone, "", true, ""},
		{"pending", on, model.NoteTranscriptionPending, "", false, ""},
		{"transcription off", off, model.NoteTranscriptionDone, "spoken words", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user, c := newLoggedIn(t, tt.app)
			n := richNote(t, tt.app, user.ID, tt.status, tt.body)

			item := c.Get("/controls/notes").RequireStatus(http.StatusOK).Doc().Find("#note-" + n.ID)
			require.Equal(t, 1, item.Length())
			require.Equal(t, 2, item.Find(".note-photos img").Length())
			require.Equal(t, fullPrompt, item.Find("button[aria-label='Delete note']").AttrOr("data-action-prompt-value", ""))
			require.Equal(t, "/controls/notes/"+n.ID+"/edit", item.Find("a[aria-label='Edit note']").AttrOr("href", ""))
			require.Equal(t, []string{"#essay", "#inline (written in the text)"}, texts(item, ".nt-tags li"))
			require.Equal(t, []string{"/controls/notes?q=%23essay", "/controls/notes?q=%23inline"}, hrefs(item, ".nt-tags a"))
			require.Equal(t, []string{sourceOf(t, tt.app, n).Title}, texts(item, "[data-testid=note-source]"))
			status := strings.TrimSpace(item.Find("[data-testid=note-status]").Text())
			if tt.status == model.NoteTranscriptionPending {
				require.Equal(t, "Transcribing…", status)
			} else {
				require.Empty(t, status)
			}

			again := item.Find("[data-testid=note-again]")
			if !tt.wantAgain {
				require.Zero(t, again.Length())
				return
			}

			require.Equal(t, n.ID, again.Find("input[name=note_id]").AttrOr("value", ""))

			confirm, has := again.Attr("hx-confirm")
			require.Equal(t, tt.wantConfirm != "", has)
			require.Equal(t, tt.wantConfirm, confirm)
		})
	}
}

func TestNotesPages_CapturePage(t *testing.T) {
	t.Parallel()

	on := notesPagesApp(t, sttOn)
	off := notesPagesApp(t, sttOff)

	attr := func(doc *goquery.Document, name string) (string, bool) {
		return doc.Find("main.capture").Attr("data-notes-capture-" + name + "-value")
	}

	tests := []struct {
		name, query string
		app         *e2e.App
		dictate     bool
		start       string
	}{
		{"on", "", on, true, ""},
		{"on, voice", "?start=voice", on, true, "voice"},
		{"on, photo", "?start=photo", on, true, "photo"},
		{"on, nonsense", "?start=nonsense", on, true, ""},
		{"off", "", off, false, ""},
		{"off, voice is not offered", "?start=voice", off, false, ""},
		{"off, photo", "?start=photo", off, false, "photo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := loginAs(t, tt.app, newUser(t, tt.app))
			doc := c.Get("/controls/notes/new" + tt.query).RequireStatus(http.StatusOK).Doc()

			buttons := texts(doc, ".capture-modes button")
			if tt.dictate {
				require.Equal(t, []string{"Dictate", "Photo", "Photo + dictate", "Type"}, buttons)
				require.Contains(t, doc.Find(".capture-disclosure").Text(), "sent to the speech-to-text provider")
			} else {
				require.Equal(t, []string{"Photo", "Type"}, buttons)
				require.Zero(t, doc.Find(".capture-disclosure").Length())
			}

			start, _ := attr(doc, "start")
			require.Equal(t, tt.start, start)

			secs, _ := attr(doc, "max-seconds")
			require.Equal(t, "600", secs)
			dim, _ := attr(doc, "max-dimension")
			require.Equal(t, "2048", dim)

			worker, _ := attr(doc, "worker")
			u, err := url.Parse(worker)
			require.NoError(t, err)
			require.Equal(t, "/controls/notes/sw.js", u.Path)
			require.Equal(t, "8388608", u.Query().Get("audio_max"))
			require.Equal(t, "33554432", u.Query().Get("capture_max"))
			require.NotEmpty(t, u.Query().Get("image_max"))
		})
	}

	t.Run("there is no focus", func(t *testing.T) {
		t.Parallel()

		c := loginAs(t, off, newUser(t, off))
		for _, page := range []string{"/controls/notes/new", "/controls/notes"} {
			doc := c.Get(page).Doc()
			require.Zero(t, doc.Find(`[data-testid="focus"]`).Length(), page)
			require.NotContains(t, doc.Text(), "Now writing", page)
		}
	})
}

func TestNotesPages_CapturePageSharedCard(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	user, c := newLoggedIn(t, app)
	rich := richNote(t, app, user.ID, model.NoteTranscriptionDone, "> quoted\n>\n> words")
	foreign := mustNote(t, app, newUser(t, app).ID, factory.WithNoteBody("not yours"))

	shared := func(id string) (string, bool) {
		return c.Get("/controls/notes/new?shared=" + id).RequireStatus(http.StatusOK).Doc().
			Find("main.capture").Attr("data-notes-capture-shared-value")
	}

	raw, ok := shared(rich.ID)
	require.True(t, ok)

	var card map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &card))
	require.Equal(t, map[string]any{
		"noteId": rich.ID, "text": "quoted\n\nwords", "photos": float64(2), "voice": true,
		// the page doesn't say which posts have it
		"deletePrompt": "Delete this note, its recording and 2 photos? This can't be undone.",
	}, card)

	for name, id := range map[string]string{
		"another user's note": foreign.ID, "not an id": "nope", "an unknown note": "0c1a7c3e-0000-4000-8000-000000000000", "empty": "",
	} {
		_, ok := shared(id)
		require.False(t, ok, name)
	}
}

func TestNotesPages_Picker(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	book := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)
	bare := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	mustNote(t, app, user.ID, factory.WithNoteBody("alpha apple"), factory.WithNoteTags("essay"), factory.CapturedAt(day(1)))
	bravo := mustNote(t, app, user.ID, factory.WithNoteBody("> bravo banana"), factory.WithNoteTags("essay", "walk"),
		factory.WithNoteTextTags("quote"), factory.FromSource(book.ID), factory.CapturedAt(day(3)))
	mustNote(t, app, user.ID, factory.WithNoteBody("> charlie cherry\n"), factory.FromSource(bare.ID), factory.CapturedAt(day(5)))
	mustNote(t, app, user.ID, factory.WithNoteBody("delta date"), factory.WithNoteTags("secret"), factory.CapturedAt(day(7)))
	mustNote(t, app, newUser(t, app).ID, factory.WithNoteBody("foreign fig"), factory.WithNoteTags("essay"))

	picker := func(query string) *goquery.Document {
		return c.Get("/controls/notes/picker" + query).RequireStatus(http.StatusOK).Doc()
	}
	bodies := func(doc *goquery.Document) []string { return texts(doc, "[data-testid=picker-note] .np-text") }

	require.Equal(t, []string{"delta date", "charlie cherry", "bravo banana", "alpha apple"}, bodies(picker("")))

	tests := []struct {
		name, query string
		want        []string
	}{
		{"tag", "?q=%23essay", []string{"bravo banana", "alpha apple"}},
		{"search", "?q=apple", []string{"alpha apple"}},
		{"search box tags", "?q=%23essay+-%23walk", []string{"alpha apple"}},
		{"search box -tag", "?q=-%23walk", []string{"delta date", "charlie cherry", "alpha apple"}},
		{"nothing", "?q=zzz", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, bodies(picker(tt.query)))
		})
	}

	require.Equal(t, []string{"No notes match."}, texts(picker("?q=zzz"), ".np-list .field-help"))

	// what Insert and Append put into the post: a quote gets its attribution line
	doc := picker("")
	text := func(i int) string {
		return doc.Find("[data-testid=picker-note]").Eq(i).Find(".np-acts button").First().AttrOr("data-text", "")
	}
	require.Equal(t, "> charlie cherry\n\n— *"+bare.Title+"*", text(1))
	require.Equal(t, "> bravo banana\n\n— *"+book.Title+"*", text(2))
	require.Equal(t, "alpha apple", text(3))

	card := doc.Find("[data-testid=picker-note]").Eq(2)
	buttons := card.Find(".np-acts button")
	labels := buttons.Map(func(_ int, b *goquery.Selection) string { return b.AttrOr("aria-label", "") })
	require.Equal(t, []string{"Insert", "Append", "Delete"}, labels)
	require.Equal(t, text(2), buttons.Eq(1).AttrOr("data-text", ""))
	require.Equal(t, "/controls/notes/"+bravo.ID+"/edit", card.Find(".np-acts a[aria-label=Edit]").AttrOr("href", ""))
	require.Equal(t, "_blank", card.Find(".np-acts a[aria-label=Edit]").AttrOr("target", ""))
	require.Equal(t, "note_delete", buttons.Eq(2).AttrOr("data-action-action-value", ""))
	require.Contains(t, buttons.Eq(2).AttrOr("data-action-prompt-value", ""), "Delete this note")
	require.Equal(t, []string{book.Title}, texts(card, ".meta"))

	// each card shows all the note's tags, set by hand or written in the text
	require.Equal(t, []string{"#essay", "#walk", "#quote"}, texts(card, ".np-tags button"))
	require.Equal(t, "walk", card.Find(".np-tags button").Eq(1).AttrOr("data-tag", ""))
	require.Zero(t, doc.Find("[data-testid=picker-note]").Eq(1).Find(".np-tags").Length())

	// under the search box, the most used tags
	require.Equal(t, []string{"#essay", "#quote", "#secret", "#walk"}, texts(doc, "ul.np-top button"))
	require.Zero(t, doc.Find("[data-testid=picker-note]").Eq(3).Find(".meta").Length())

	// the panel has its own search box: the fragment has no source filter
	require.Zero(t, doc.Find("nav.nt-bar, details.np-sources, form.np-search").Length())
}

func TestNotesPages_PickerCursor(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff, e2e.WithEnv("PAGE_SIZE", "2"))
	user, c := newLoggedIn(t, app)

	for i, b := range []string{"day one", "day two", "day three"} {
		mustNote(t, app, user.ID, factory.WithNoteBody(b), factory.WithNoteTags("x"), factory.CapturedAt(day(i+1)))
	}

	doc := c.Get("/controls/notes/picker?q=%23x").Doc()
	require.Equal(t, []string{"day three", "day two"}, texts(doc, "[data-testid=picker-note] .np-text"))

	more := doc.Find(".load-more a").AttrOr("href", "")
	u, err := url.Parse(more)
	require.NoError(t, err)
	require.Equal(t, "/controls/notes/picker", u.Path)
	require.Equal(t, []string{"#x"}, u.Query()["q"])

	doc = c.Get(more).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"day one"}, texts(doc, "[data-testid=picker-note] .np-text"))
	require.Zero(t, doc.Find(".load-more").Length())
}

func TestNotesPages_ControlsSection(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	user, c := newLoggedIn(t, app)

	section := func() *goquery.Selection {
		return c.Get("/controls/").RequireStatus(http.StatusOK).Doc().Find("#controls-notes-title").Parent()
	}

	s := section()
	require.Equal(t, "No notes yet.", strings.TrimSpace(s.Find("p.field-help").Text()))
	require.Zero(t, s.Find(".controls-list").Length())
	require.NotContains(t, s.Text(), "Inbox")

	mustNote(t, app, user.ID, factory.WithNoteBody("tagged"), factory.WithNoteTags("a"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteBody("untagged one"), factory.CapturedAt(day(2)))

	mustNote(t, app, user.ID, factory.WithNoteBody(""), factory.CapturedAt(day(3)))
	notes := make([]*model.Note, 0, 5)

	for i := 4; i <= 8; i++ {
		notes = append(notes, mustNote(t, app, user.ID, factory.WithNoteBody("note on "+string(rune('0'+i))), factory.WithNoteTags("a"), factory.CapturedAt(day(i))))
	}

	s = section()
	require.Zero(t, s.Find("p.field-help").Length())

	// the five newest, newest first, each a link to its edit page
	require.Equal(t, []string{"note on 8", "note on 7", "note on 6", "note on 5", "note on 4"}, texts(s, "a.note-excerpt"))
	require.Equal(t, "/controls/notes/"+notes[4].ID+"/edit", s.Find("a.note-excerpt").First().AttrOr("href", ""))
	require.Equal(t, "/controls/notes", linkHref(s, "All notes"))
	require.Equal(t, "/controls/notes/new", linkHref(s, "Capture"))

	// a note with no text is told by its kind
	for i := 4; i <= 8; i++ {
		c.PostJSON("/controls/action/note_delete", map[string]string{"noteId": notes[i-4].ID}).RequireStatus(http.StatusOK)
	}

	require.Equal(t, []string{"Recording or photo", "untagged one", "tagged"}, texts(section(), "a.note-excerpt"))
}

func linkHref(s *goquery.Selection, text string) string {
	return s.Find("p.acts a").FilterFunction(func(_ int, a *goquery.Selection) bool { return strings.TrimSpace(a.Text()) == text }).AttrOr("href", "")
}

// The capture box takes tags, with or without #, and offers the tags of the
// newest notes, newest first, one tap away.
func TestNotesPages_CaptureTags(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	user, c := newLoggedIn(t, app)

	mustNote(t, app, user.ID, factory.WithNoteBody("old"), factory.WithNoteTags("old"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteBody("new #inline"), factory.WithNoteTags("walk"), factory.WithNoteTextTags("inline"), factory.CapturedAt(day(3)))

	doc := c.Get("/controls/notes?q=zzz").Doc()
	require.Equal(t, []string{"#walk", "#inline", "#old"}, texts(doc, "ul[aria-label='Recent tags'] button"), "the newest notes, whatever the list shows")

	resp := c.PostForm("/controls/form/note_create", url.Values{"body": {"captured"}, "tags": {"#Essay, walk  #walk"}}).RequireStatus(http.StatusOK)
	require.Equal(t, "true", resp.Header.Get("HX-Refresh"))

	got := userNotesByBody(t, app, user.ID)["captured"]
	require.NotNil(t, got)
	require.Equal(t, []string{"essay", "walk"}, got.Tags)

	resp = c.PostForm("/controls/form/note_create", url.Values{"body": {"bad"}, "tags": {"no!"}}).RequireStatus(http.StatusOK)
	require.Contains(t, resp.Body, "Not saved")
	require.NotContains(t, userNotesByBody(t, app, user.ID), "bad")
}

// The note forms save the source chosen from the author's own, or none.
func TestNotesPages_NoteFormsSaveTheSource(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	book := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	resp := c.PostForm("/controls/form/note_create", url.Values{"body": {"from a book"}, "source_id": {book.ID}}).RequireStatus(http.StatusOK)
	require.Equal(t, "true", resp.Header.Get("HX-Refresh"))
	created := userNotesByBody(t, app, user.ID)["from a book"]
	require.Equal(t, book.ID, *created.SourceID)

	c.PostForm("/controls/form/note_create", url.Values{"body": {"no book"}, "source_id": {""}}).RequireStatus(http.StatusOK)
	require.Nil(t, userNotesByBody(t, app, user.ID)["no book"].SourceID)

	path := "/controls/form/note_edit/" + created.ID
	note := func() *model.Note { return testutil.Must(repo.New(app.DB).NoteByID(ctx, created.ID))(t) }

	resp = c.PostForm(path, url.Values{"body": {"from a book"}, "source_id": {""}}).RequireStatus(http.StatusOK)
	require.Contains(t, resp.Body, "Saved")
	require.Nil(t, note().SourceID)

	c.PostForm(path, url.Values{"body": {"from a book"}, "source_id": {book.ID}}).RequireStatus(http.StatusOK)
	require.Equal(t, book.ID, *note().SourceID)

	// an unknown id
	resp = c.PostForm(path, url.Values{"body": {"from a book"}, "source_id": {"0c1a7c3e-0000-4000-8000-000000000000"}}).RequireStatus(http.StatusOK)
	require.Contains(t, resp.Body, "Not saved")
	require.Equal(t, book.ID, *note().SourceID)

	// the old source form is gone
	c.PostForm("/controls/form/note_source/"+created.ID, url.Values{"title": {"x"}}).RequireStatus(http.StatusNotFound)
}

// userNotesByBody are the author's notes by their text.
func userNotesByBody(t *testing.T, app *e2e.App, userID string) map[string]*model.Note {
	t.Helper()

	rows, err := repo.New(app.DB).ListNotes(context.Background(), repo.NoteFilter{UserID: userID, Limit: 50})
	require.NoError(t, err)

	out := map[string]*model.Note{}
	for _, r := range rows {
		out[r.Note.Body] = r.Note
	}

	return out
}
