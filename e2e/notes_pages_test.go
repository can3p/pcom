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

// chip is a tag chip of a tag bar.
type chip struct{ state, href string }

func chips(doc *goquery.Document) map[string]chip {
	out := map[string]chip{}

	doc.Find("nav.nt-bar li.nt-chip").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a")
		href, _ := a.Attr("href")
		state, _ := s.Attr("data-state")
		out[strings.Fields(a.Text())[0]] = chip{state: state, href: href}
	})

	return out
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
		{"tag", "?tag=essay", []string{"bravo banana", "alpha apple"}},
		{"two tags", "?tag=essay&tag=walk", []string{"bravo banana"}},
		{"not", "?not=essay", []string{"echo elder", "delta date", "charlie Cherry"}},
		{"tag and not", "?tag=essay&not=walk", []string{"alpha apple"}},
		{"search box tag", "?q=%23Secret", []string{"echo elder"}},
		{"search box tags and text", "?q=%23essay+-%23walk+apple", []string{"alpha apple"}},
		{"search box tag left out", "?q=-%23walk+-%23secret", []string{"charlie Cherry", "alpha apple"}},
		{"inbox", "?inbox=1", []string{"charlie Cherry"}},
		{"unused", "?unused=1", []string{"echo elder", "charlie Cherry", "bravo banana", "alpha apple"}},
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
	require.NotContains(t, texts(c.Get("/controls/notes?inbox=1").Doc(), "[data-testid=note] .meta")[0], "used in a post")
}

func TestNotesPages_TagBarViewsAndSources(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	src := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	mustNote(t, app, user.ID, factory.WithNoteTags("essay"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteTags("essay", "walk"), factory.CapturedAt(day(3)))
	mustNote(t, app, user.ID, factory.WithNoteTags("secret", "walk"), factory.FromSource(src.ID), factory.CapturedAt(day(5)))

	doc := c.Get("/controls/notes?tag=essay&not=walk").RequireStatus(http.StatusOK).Doc()
	got := chips(doc)
	// include goes to exclude, exclude to off, off to include; the other tags stay
	require.Equal(t, map[string]chip{
		"#essay":  {"include", "/controls/notes?not=walk&not=essay"},
		"#walk":   {"exclude", "/controls/notes?tag=essay"},
		"#secret": {"off", "/controls/notes?not=walk&tag=essay&tag=secret"},
	}, got)
	require.Equal(t, []string{"#essay 2 (included)", "#walk 2 (excluded)", "#secret 1"}, texts(doc, "nav.nt-bar li.nt-chip a"))

	// views: a filter keeps the others, and the active one toggles off
	doc = c.Get("/controls/notes?tag=essay").Doc()
	href, _ := linkTo(doc, ".nt-views ul[aria-label=Views] a", "Inbox")
	require.Equal(t, "/controls/notes?inbox=1&tag=essay", href)
	href, _ = linkTo(doc, ".nt-views ul[aria-label=Views] a", "Unused")
	require.Equal(t, "/controls/notes?tag=essay&unused=1", href)
	require.Zero(t, doc.Find("[aria-current]").Length())

	doc = c.Get("/controls/notes?inbox=1&unused=1").Doc()
	href, _ = linkTo(doc, "ul[aria-label=Views] a[aria-current]", "Inbox")
	require.Equal(t, "/controls/notes?unused=1", href)
	href, _ = linkTo(doc, "ul[aria-label=Views] a[aria-current]", "Unused")
	require.Equal(t, "/controls/notes?inbox=1", href)

	// sources: a count, a link that selects it and, selected, one that clears it
	doc = c.Get("/controls/notes?tag=walk").Doc()
	href, ok := linkTo(doc, "ul[aria-label=Sources] a", src.Title+" 1")
	require.True(t, ok)
	require.Equal(t, "/controls/notes?source="+src.ID+"&tag=walk", href)

	doc = c.Get("/controls/notes?tag=walk&source=" + src.ID).Doc()
	href, ok = linkTo(doc, "ul[aria-label=Sources] a[aria-current]", src.Title)
	require.True(t, ok)
	require.Equal(t, "/controls/notes?tag=walk", href)
	require.Equal(t, src.ID, doc.Find("form.nt-search input[name=source]").AttrOr("value", ""))
	require.Equal(t, "walk", doc.Find("form.nt-search input[name=tag]").AttrOr("value", ""))
}

func TestNotesPages_CursorPages(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff, e2e.WithEnv("PAGE_SIZE", "2"))
	user, c := newLoggedIn(t, app)

	for i, body := range []string{"one", "two", "three"} {
		mustNote(t, app, user.ID, factory.WithNoteBody(body), factory.WithNoteTags("x"), factory.CapturedAt(day(i+1)))
	}

	mustNote(t, app, user.ID, factory.WithNoteBody("untagged"), factory.CapturedAt(day(9)))

	doc := c.Get("/controls/notes?tag=x").Doc()
	require.Equal(t, []string{"three", "two"}, texts(doc, "[data-testid=note] .note-body"))

	more := doc.Find(".load-more a").AttrOr("href", "")
	u, err := url.Parse(more)
	require.NoError(t, err)
	require.Equal(t, "/controls/notes", u.Path)
	require.Equal(t, []string{"x"}, u.Query()["tag"], "the next page keeps the filter")
	require.NotEmpty(t, u.Query().Get("cursor"))

	doc = c.Get(more).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"one"}, texts(doc, "[data-testid=note] .note-body"))
	require.Zero(t, doc.Find(".load-more").Length())

	// the last page of the whole list is one short too
	doc = c.Get("/controls/notes").Doc()
	require.Equal(t, []string{"untagged", "three"}, texts(doc, "[data-testid=note] .note-body"))
}

// Once a recording on the list is transcribed, its poll answers with the
// whole list item, suggestions included when the list shows them.
func TestNotesPages_StatusPollOnTheListAnswersTheItem(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOn)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)

	mustNote(t, app, user.ID, factory.WithNoteTags("essay"))
	audio := testutil.Must(factory.NoteAudio(ctx, app.DB, user.ID))(t)
	n := mustNote(t, app, user.ID, factory.WithNoteBody("an essay draft"), factory.WithRecording(audio.ID),
		factory.WithNoteTranscription(model.NoteTranscriptionDone))

	poll := "/controls/notes/" + n.ID + "/status?view=list"

	doc := c.Get(poll + "&suggest=1").RequireStatus(http.StatusOK).Doc()
	item := doc.Find("article#note-" + n.ID)
	require.Equal(t, 1, item.Length())
	require.Equal(t, []string{"an essay draft"}, texts(item, ".note-body"))
	require.Equal(t, []string{"+ #essay"}, texts(item, ".nt-suggest button"))

	doc = c.Get(poll).RequireStatus(http.StatusOK).Doc()
	require.Equal(t, 1, doc.Find("article#note-"+n.ID).Length())
	require.Zero(t, doc.Find(".nt-suggest").Length())
}

func TestNotesPages_ManageScreen(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)

	doc := c.Get("/controls/notes/manage").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, []string{"No tags yet."}, texts(doc, "#note-tags-title ~ ul li"))
	require.Equal(t, []string{"No sources yet. Tag a selection of notes with a source from the notes list."}, texts(doc, "#note-sources-title ~ ul li"))

	one := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceAuthor("Ann Author")))(t)
	two := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)
	mustNote(t, app, user.ID, factory.WithNoteTags("essay", "walk"), factory.FromSource(one.ID))
	mustNote(t, app, user.ID, factory.WithNoteTags("essay"), factory.FromSource(one.ID))
	mustNote(t, app, user.ID, factory.WithNoteTags("secret"), factory.FromSource(two.ID))
	c.PostJSON("/controls/action/note_focus", map[string]string{"tag": "walk"}).RequireStatus(http.StatusOK)

	doc = c.Get("/controls/notes/manage").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "Tags and sources", doc.Find("h1").Text())
	require.Equal(t, []string{
		"#essay 2 notes rename focus",
		"#secret 1 note rename focus",
		"#walk 1 note, focus rename",
	}, texts(doc, "[data-testid=tag-row]"))
	require.Equal(t, "Rename #essay to (a name that exists merges the two tags)",
		doc.Find("[data-testid=tag-row] button[aria-label='Rename #essay']").AttrOr("data-action-prompt-value", ""))

	require.Equal(t, 2, doc.Find("[data-testid=source-row]").Length())

	row := doc.Find("[data-testid=source-row]").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("a").First().AttrOr("href", "") == "/controls/notes?source="+one.ID
	})
	require.Equal(t, 1, row.Length())
	require.Equal(t, one.Title, row.Find("a").First().Text())
	require.Contains(t, texts(row, ".meta"), "Ann Author")
	require.Contains(t, texts(row, ".meta"), "2 notes")
	// merging offers the other source only
	require.Equal(t, []string{two.Title}, texts(row, "select option"))
	require.Equal(t, one.ID, row.Find("form.nt-merge input[name=from]").AttrOr("value", ""))
	require.Equal(t, "Merge "+one.Title+" into the source you chose? Its notes move there.",
		row.Find("form.nt-merge button").AttrOr("data-confirm-message-value", ""))
}

// richNote is a note with a recording in the given state, two photos, a
// source, a locator and a post that has its text.
func richNote(t *testing.T, app *e2e.App, userID string, status model.NoteTranscriptionStatus, body string) *model.Note {
	t.Helper()

	ctx := context.Background()
	src := testutil.Must(factory.NoteSource(ctx, app.DB, userID))(t)
	audio := testutil.Must(factory.NoteAudio(ctx, app.DB, userID))(t)
	n := mustNote(t, app, userID, factory.WithNoteBody(body), factory.WithNoteTags("essay"), factory.WithNoteTextTags("essay", "inline"),
		factory.WithRecording(audio.ID), factory.WithNoteTranscription(status), factory.FromSource(src.ID), factory.WithNoteLocator("page 42"))

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

			require.Equal(t, "page 42", doc.Find("#note-locator").AttrOr("value", ""))
			require.Equal(t, "/controls/notes?source="+*n.SourceID, doc.Find("p.meta a").AttrOr("href", ""))

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
		require.Zero(t, doc.Find("#note-locator").Length())
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
			require.Equal(t, fullPrompt, item.Find("button.btn-danger").AttrOr("data-action-prompt-value", ""))
			require.Equal(t, "/controls/notes/"+n.ID+"/edit", item.Find(".acts a").AttrOr("href", ""))
			require.Equal(t, []string{"#essay", "#inline (written in the text)"}, texts(item, ".nt-tags li"))
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

	t.Run("the focus tag shows", func(t *testing.T) {
		t.Parallel()

		c := loginAs(t, off, newUser(t, off))
		require.Contains(t, c.Get("/controls/notes/new").Doc().Find("[data-testid=focus]").Text(), "No focus tag")

		c.PostJSON("/controls/action/note_focus", map[string]string{"tag": "essay"}).RequireStatus(http.StatusOK)
		require.Equal(t, []string{"#essay"}, texts(c.Get("/controls/notes/new").Doc(), ".capture-chip"))
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
	book := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID, factory.WithSourceAuthor("Ann Author")))(t)
	bare := testutil.Must(factory.NoteSource(ctx, app.DB, user.ID))(t)

	mustNote(t, app, user.ID, factory.WithNoteBody("alpha apple"), factory.WithNoteTags("essay"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteBody("> bravo banana"), factory.WithNoteTags("essay", "walk"),
		factory.FromSource(book.ID), factory.WithNoteLocator("42"), factory.CapturedAt(day(3)))
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
		{"tag", "?tag=essay", []string{"bravo banana", "alpha apple"}},
		{"not", "?not=walk", []string{"delta date", "charlie cherry", "alpha apple"}},
		{"search", "?q=apple", []string{"alpha apple"}},
		{"search box tags", "?q=%23essay+-%23walk", []string{"alpha apple"}},
		{"source", "?source=" + book.ID, []string{"bravo banana"}},
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
		return doc.Find("[data-testid=picker-note]").Eq(i).Find("button").First().AttrOr("data-text", "")
	}
	require.Equal(t, "> charlie cherry\n\n— *"+bare.Title+"*", text(1))
	require.Equal(t, "> bravo banana\n\n— Ann Author, *"+book.Title+"*, p. 42", text(2))
	require.Equal(t, "alpha apple", text(3))

	buttons := doc.Find("[data-testid=picker-note]").Eq(2).Find("button")
	require.Equal(t, []string{"Insert", "Append"}, texts(buttons.Parent(), "button"))
	require.Equal(t, text(2), buttons.Eq(1).AttrOr("data-text", ""))
	require.Equal(t, []string{book.Title + ", 42"}, texts(doc.Find("[data-testid=picker-note]").Eq(2), ".meta"))
	require.Zero(t, doc.Find("[data-testid=picker-note]").Eq(3).Find(".meta").Length())

	// chips and filters load the picker, not the notes page
	got := chips(picker("?tag=essay&not=walk"))
	require.Equal(t, chip{"include", "/controls/notes/picker?not=walk&not=essay"}, got["#essay"])
	require.Equal(t, chip{"exclude", "/controls/notes/picker?tag=essay"}, got["#walk"])
	require.Equal(t, "#np-body", picker("?tag=essay").Find("nav.nt-bar li.nt-chip a").First().AttrOr("hx-target", ""))

	// sources
	doc = picker("?source=" + book.ID)
	_, open := doc.Find("details.np-sources").Attr("open")
	require.True(t, open)
	require.Equal(t, "/controls/notes/picker", doc.Find("details.np-sources a.is-active").AttrOr("href", ""))
	require.Equal(t, "/controls/notes/picker?source="+bare.ID, doc.Find("details.np-sources a:not(.is-active)").AttrOr("href", ""))
	_, open = picker("").Find("details.np-sources").Attr("open")
	require.False(t, open)
}

func TestNotesPages_PickerCursor(t *testing.T) {
	t.Parallel()

	app := notesPagesApp(t, sttOff, e2e.WithEnv("PAGE_SIZE", "2"))
	user, c := newLoggedIn(t, app)

	for i, b := range []string{"day one", "day two", "day three"} {
		mustNote(t, app, user.ID, factory.WithNoteBody(b), factory.WithNoteTags("x"), factory.CapturedAt(day(i+1)))
	}

	doc := c.Get("/controls/notes/picker?tag=x").Doc()
	require.Equal(t, []string{"day three", "day two"}, texts(doc, "[data-testid=picker-note] .np-text"))

	more := doc.Find(".load-more a").AttrOr("href", "")
	u, err := url.Parse(more)
	require.NoError(t, err)
	require.Equal(t, "/controls/notes/picker", u.Path)
	require.Equal(t, []string{"x"}, u.Query()["tag"])

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
	require.Equal(t, "The Inbox is empty", strings.TrimSpace(s.Find("p.field-help").Text()))
	require.Zero(t, s.Find(".controls-list").Length())

	mustNote(t, app, user.ID, factory.WithNoteBody("tagged"), factory.WithNoteTags("a"), factory.CapturedAt(day(1)))
	mustNote(t, app, user.ID, factory.WithNoteBody("untagged one"), factory.CapturedAt(day(2)))

	s = section()
	require.Equal(t, "1 untagged note waits in the Inbox", strings.TrimSpace(s.Find("p.field-help").Text()))
	require.Equal(t, "/controls/notes?inbox=1", s.Find("p.field-help a").AttrOr("href", ""))

	mustNote(t, app, user.ID, factory.WithNoteBody(""), factory.CapturedAt(day(3)))
	notes := make([]*model.Note, 0, 5)

	for i := 4; i <= 8; i++ {
		notes = append(notes, mustNote(t, app, user.ID, factory.WithNoteBody("note on "+string(rune('0'+i))), factory.WithNoteTags("a"), factory.CapturedAt(day(i))))
	}

	s = section()
	require.Equal(t, "2 untagged notes wait in the Inbox", strings.TrimSpace(s.Find("p.field-help").Text()))

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
