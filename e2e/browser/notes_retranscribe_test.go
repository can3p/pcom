//go:build browser

// "Transcribe again as…" on a note with a recording, on the list and on the
// edit page: the picked language is sent with the recording again, the text
// is replaced after a confirmation when the note has text, a refusal shows in
// place, and notes without a recording or without a backend have no control.
package browser_test

import (
	"context"
	"sync/atomic"
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

// The Russian transcript of the stub in testdata/wiremock/notes_capture, which
// answers a request with language=ru; it counts 20 seconds against the limit.
const (
	retranscribeRussian = "Гуляя вдоль реки, я думал об эссе."
	retranscribeConfirm = "Replace the edited text with a new transcript?"
)

// setBody writes body as the note's text, as an author's save would.
func setBody(t *testing.T, app *e2e.App, id, body string) {
	t.Helper()

	store := repo.New(app.DB)
	n, err := store.NoteByID(context.Background(), id)
	require.NoError(t, err)

	n.Body = body
	require.NoError(t, store.SaveNote(context.Background(), n, "body"))
}

// transcribeAs picks the language in the control of the page and sends it.
func transcribeAs(t *testing.T, page playwright.Page, language string) {
	t.Helper()

	form := page.GetByTestId("note-again")
	_, err := form.GetByLabel("Transcribe again as…").SelectOption(playwright.SelectOptionValues{Labels: &[]string{language}})
	require.NoError(t, err)
	require.NoError(t, form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Transcribe"}).Click())
}

// confirmed waits for the confirmation dialog and checks what it asks.
func confirmed(t *testing.T, dialogs <-chan string) {
	t.Helper()

	select {
	case msg := <-dialogs:
		require.Equal(t, retranscribeConfirm, msg)
	case <-time.After(10 * time.Second):
		t.Fatal("no confirmation before replacing text")
	}
}

func TestNotesRetranscribe(t *testing.T) {
	t.Parallel()

	// one minute a day: the Russian stub counts 20 seconds a time, so the
	// third request is over the limit
	app := sttApp(t, e2e.WithEnv("STT_MINUTES_PER_DAY", "1"))
	user := browser.NewUser(t, app)

	capture := capturePage(t, app, user, "/controls/notes/new")
	require.NoError(t, capture.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
	record(t, capture)

	n := waitNotes(t, app, user.ID, 1)[0]
	eventuallyNote(t, app, user.ID, func(m *model.Note) bool { return m.ID == n.ID && m.Body == captureTranscript })

	page := phonePage(t, app, user, false)

	var accept atomic.Bool

	dialogs := make(chan string, 8)

	page.OnDialog(func(d playwright.Dialog) {
		dialogs <- d.Message()

		if accept.Load() {
			_ = d.Accept()
		} else {
			_ = d.Dismiss()
		}
	})

	open := func(path string) {
		_, err := page.Goto(path)
		require.NoError(t, err)
	}

	t.Run("cancelling the confirmation keeps the edited text", func(t *testing.T) {
		setBody(t, app, n.ID, "My own words")
		accept.Store(false)

		open("/controls/notes")
		transcribeAs(t, page, "Russian")

		confirmed(t, dialogs)

		require.Never(t, func() bool {
			return noteIn(t, app, n.ID).Body != "My own words"
		}, 4*time.Second, 200*time.Millisecond)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToContainText("My own words"))
	})

	t.Run("on the list the picked language replaces the text", func(t *testing.T) {
		accept.Store(true)

		open("/controls/notes")
		transcribeAs(t, page, "Russian")

		confirmed(t, dialogs)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToContainText(retranscribeRussian,
			playwright.LocatorAssertionsToContainTextOptions{Timeout: new(30000.0)}))
		require.Equal(t, retranscribeRussian, noteIn(t, app, n.ID).Body)
	})

	t.Run("on the edit page the text box takes the new transcript", func(t *testing.T) {
		setBody(t, app, n.ID, "Typed since")

		open("/controls/notes/" + n.ID + "/edit")
		transcribeAs(t, page, "Russian")

		confirmed(t, dialogs)
		require.NoError(t, browser.Expect.Locator(page.GetByLabel("Note", playwright.PageGetByLabelOptions{Exact: new(true)})).ToHaveValue(retranscribeRussian,
			playwright.LocatorAssertionsToHaveValueOptions{Timeout: new(30000.0)}))
		require.Equal(t, retranscribeRussian, noteIn(t, app, n.ID).Body)
	})

	t.Run("the daily limit is shown in place", func(t *testing.T) {
		setBody(t, app, n.ID, "Kept text")

		open("/controls/notes")
		transcribeAs(t, page, "Russian")

		confirmed(t, dialogs)

		form := page.GetByTestId("note-again")
		require.NoError(t, browser.Expect.Locator(form).ToContainText("of your 1 minutes of transcription"))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToContainText("Kept text"))
		require.Equal(t, "Kept text", noteIn(t, app, n.ID).Body)
	})
}

func TestNotesRetranscribe_NoControl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a note without a recording", func(t *testing.T) {
		app := sttApp(t)
		user := browser.NewUser(t, app)
		note := mustNote(t, app, user.ID, factory.WithNoteBody("Typed only"))

		page := browser.Page(t, app, browser.As(user))

		_, err := page.Goto("/controls/notes")
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToContainText("Typed only"))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToHaveCount(0))

		_, err = page.Goto("/controls/notes/" + note.ID + "/edit")
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)})).ToBeVisible())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToHaveCount(0))
	})

	t.Run("no speech-to-text backend", func(t *testing.T) {
		app := e2e.Start(t, e2e.WithRealAssets())
		user := browser.NewUser(t, app)

		rec, err := factory.NoteRecording(ctx, app.DB, user.ID)
		require.NoError(t, err)

		note := mustNote(t, app, user.ID, factory.WithNoteBody("Spoken"), factory.WithRecording(rec.ID))

		page := browser.Page(t, app, browser.As(user))

		_, err = page.Goto("/controls/notes")
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToContainText("Spoken"))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToHaveCount(0))

		_, err = page.Goto("/controls/notes/" + note.ID + "/edit")
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)})).ToBeVisible())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToHaveCount(0))
	})
}

// A note opened for editing while its recording is transcribed has no control
// yet; the poll that brings the text brings the control with it.
func TestNotesRetranscribe_EditPageWhilePending(t *testing.T) {
	t.Parallel()

	app := sttApp(t)
	user := browser.NewUser(t, app)

	capture := capturePage(t, app, user, "/controls/notes/new")
	require.NoError(t, capture.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
	record(t, capture)

	n := waitNotes(t, app, user.ID, 1)[0]

	page := browser.Page(t, app, browser.As(user))

	_, err := page.Goto("/controls/notes/" + n.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-status")).ToHaveText("Transcribing…"))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToHaveCount(0))

	require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-again")).ToBeVisible(
		playwright.LocatorAssertionsToBeVisibleOptions{Timeout: new(30000.0)}))
	require.NoError(t, browser.Expect.Locator(page.GetByLabel("Note", playwright.PageGetByLabelOptions{Exact: new(true)})).ToHaveValue(captureTranscript))
}
