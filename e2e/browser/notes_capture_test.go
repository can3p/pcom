//go:build browser

// The capture page (/controls/notes/new) on a phone: dictating, a photo, a
// photo with a spoken caption and typed text, each saved under the focus;
// ?start=; captures made offline uploaded when the network returns; and
// deleting the last capture, uploaded or still queued.
package browser_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

var _ = func() struct{} {
	wiremock.Register("testdata/wiremock/notes_capture")

	return struct{}{}
}()

// The transcript and key of the WireMock stub in testdata/wiremock/notes_capture.
const (
	captureTranscript = "Walking past the river I thought of the essay."
	captureSTTKey     = "notes-capture-key"
)

// waitForWorker resolves once the service worker controls the page, and fails
// after ten seconds. Playwright's polling waits evaluate strings, which the
// page's CSP refuses.
const waitForWorker = `Promise.race([
  new Promise((resolve) => {
    if (navigator.serviceWorker.controller) resolve(true)
    navigator.serviceWorker.addEventListener("controllerchange", () => resolve(true))
  }),
  new Promise((_, reject) => setTimeout(() => reject(new Error("no service worker")), 10000)),
])`

// phonePage opens a page at phone size, without navigating; mic allows the
// microphone.
func phonePage(t *testing.T, app *e2e.App, user *model.User, mic bool) playwright.Page {
	t.Helper()

	return browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Viewport = &playwright.Size{Width: 390, Height: 844}
		o.IsMobile = new(true)
		o.HasTouch = new(true)

		if mic {
			o.Permissions = []string{"microphone"}
		}
	}))
}

// openCapture goes to path and waits for the service worker to control it:
// it uploads, so the network may only go away once it runs.
func openCapture(t *testing.T, page playwright.Page, path string) {
	t.Helper()

	_, err := page.Goto(path)
	require.NoError(t, err)

	_, err = page.Evaluate(waitForWorker)
	require.NoError(t, err)
}

// capturePage opens path on a phone whose microphone is allowed.
func capturePage(t *testing.T, app *e2e.App, user *model.User, path string) playwright.Page {
	t.Helper()

	page := phonePage(t, app, user, true)
	openCapture(t, page, path)

	return page
}

// userNotes are the author's notes with their photos and recording, newest
// capture first.
func userNotes(t *testing.T, app *e2e.App, userID string) []*model.Note {
	t.Helper()

	store := repo.New(app.DB)

	rows, err := store.ListNotes(context.Background(), repo.NoteFilter{UserID: userID, ShowHidden: true, Limit: 50})
	require.NoError(t, err)

	out := make([]*model.Note, 0, len(rows))

	for _, r := range rows {
		n, err := store.NoteWithRelations(context.Background(), r.Note.ID)
		require.NoError(t, err)

		out = append(out, n)
	}

	return out
}

// eventuallyNote waits for a note of the author that ok accepts.
func eventuallyNote(t *testing.T, app *e2e.App, userID string, ok func(n *model.Note) bool) *model.Note {
	t.Helper()

	var found *model.Note

	require.Eventually(t, func() bool {
		i := slices.IndexFunc(userNotes(t, app, userID), ok)
		if i < 0 {
			return false
		}

		found = userNotes(t, app, userID)[i]

		return true
	}, 30*time.Second, 200*time.Millisecond)

	return found
}

// waitNotes waits until the author has count notes and returns them, newest
// capture first.
func waitNotes(t *testing.T, app *e2e.App, userID string, count int) []*model.Note {
	t.Helper()

	var notes []*model.Note

	require.Eventually(t, func() bool {
		notes = userNotes(t, app, userID)

		return len(notes) == count
	}, 30*time.Second, 200*time.Millisecond, "want %d notes", count)

	return notes
}

// widePNG is a photo wider than the page scales photos down to.
func widePNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	for x := range 1200 {
		img.Set(x, x%600, color.RGBA{R: 200, A: 255})
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

// takePhoto taps the button and picks the photo in the file chooser it opens.
func takePhoto(t *testing.T, page playwright.Page, button string, photo []byte) {
	t.Helper()

	chooser, err := page.ExpectFileChooser(func() error {
		return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: button, Exact: new(true)}).Click()
	})
	require.NoError(t, err)
	require.NoError(t, chooser.SetFiles([]playwright.InputFile{{Name: "wide.png", MimeType: "image/png", Buffer: photo}}))
}

// record lets the recorder run past its first second, then stops it.
func record(t *testing.T, page playwright.Page) {
	t.Helper()

	timer := page.GetByTestId("timer")
	require.NoError(t, browser.Expect.Locator(timer).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(timer).ToContainText("0:01"))
	require.NoError(t, page.Locator(".capture-rec").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Stop"}).Click())
	require.NoError(t, browser.Expect.Locator(timer).ToBeHidden())
}

func expectLast(t *testing.T, page playwright.Page, what, status string) {
	t.Helper()

	card := page.GetByTestId("last-capture")
	require.NoError(t, browser.Expect.Locator(card).ToContainText(what))
	require.NoError(t, browser.Expect.Locator(card.GetByTestId("capture-status")).ToHaveText(status, playwright.LocatorAssertionsToHaveTextOptions{Timeout: new(30000.0)}))
}

// sttApp starts the app with the WireMock speech-to-text backend, whose
// answer takes a few seconds, so a note shows its transcription pending.
func sttApp(t *testing.T, opts ...e2e.Option) *e2e.App {
	t.Helper()

	return e2e.Start(t, append([]e2e.Option{
		e2e.WithRealAssets(),
		e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", wiremock.Shared(t).Watch(t, "notes-capture")),
		e2e.WithEnv("STT_OPENAI_COMPAT_KEY", captureSTTKey),
	}, opts...)...)
}

// typeNote saves text with the Type mode.
func typeNote(t *testing.T, page playwright.Page, text string) {
	t.Helper()

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"}).Click())
	require.NoError(t, page.GetByLabel("Note").Fill(text))
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save note"}).Click())
}

func TestNotesCapture_VoicePhotoAndText(t *testing.T) {
	t.Parallel()

	app := sttApp(t, e2e.WithEnv("NOTE_PHOTO_MAX_DIMENSION", "400"))
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteTags("walk"))

	page := capturePage(t, app, user, "/controls/notes/new")
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))

	_, err := page.Goto("/controls/notes/manage")
	require.NoError(t, err)
	// The action reloads the page once saved. The row's "focus" button holds
	// the word before the click too, so wait for the reloaded row: no button,
	// and "focus" in its counts. Going on earlier races the reload.
	focusOn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Focus on #walk"})
	require.NoError(t, focusOn.Click())
	require.NoError(t, browser.Expect.Locator(focusOn).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("tag-row").Locator(".meta")).ToHaveText("1 note, focus"))

	_, err = page.Goto("/controls/notes/new")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("#walk"))
	require.NoError(t, browser.Expect.Locator(page.GetByText("sent to the speech-to-text provider")).ToBeVisible())

	underFocus := func(n *model.Note) bool { return slices.Contains(n.Tags, "walk") }
	count := 1

	t.Run("dictate", func(t *testing.T) {
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
		record(t, page)
		expectLast(t, page, "A recording of 0:0", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.NotNil(t, n.AudioUploadID)
		require.Empty(t, n.Images)
		require.True(t, underFocus(n), "tags %v", n.Tags)

		// the list shows the transcription pending, then its text
		_, err := page.Goto("/controls/notes")
		require.NoError(t, err)

		status := page.GetByTestId("note-status")
		require.NoError(t, browser.Expect.Locator(status).ToHaveText("Transcribing…"))
		require.NotContains(t, noteIn(t, app, n.ID).Body, captureTranscript)

		// both notes picked, the recording among them
		pick := func(card playwright.Locator) playwright.Locator {
			return card.GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: "Select this note"})
		}
		pending := page.GetByTestId("note").Filter(playwright.LocatorFilterOptions{Has: status})
		written := page.GetByTestId("note").Filter(playwright.LocatorFilterOptions{HasNot: page.GetByTestId("note-status")})
		selected := page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Selected notes"})

		require.NoError(t, pick(pending).Check())
		require.NoError(t, pick(written).Check())
		require.NoError(t, browser.Expect.Locator(selected).ToContainText("2 selected"))

		// the status asks again until the text is there, which replaces the
		// note in place
		require.NoError(t, browser.Expect.Locator(noteCard(page, captureTranscript)).ToBeVisible(playwright.LocatorAssertionsToBeVisibleOptions{Timeout: new(30000.0)}))
		require.NoError(t, browser.Expect.Locator(status).ToHaveText(""))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(2))
		require.NoError(t, browser.Expect.Locator(page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Notes"})).ToHaveCount(1))
		require.Equal(t, captureTranscript, noteIn(t, app, n.ID).Body)

		// the replaced note comes back unpicked, and the bar acts on the
		// boxes still ticked only
		require.NoError(t, browser.Expect.Locator(pick(noteCard(page, captureTranscript))).Not().ToBeChecked())
		require.NoError(t, browser.Expect.Locator(pick(written)).ToBeChecked())
		require.NoError(t, browser.Expect.Locator(selected).ToContainText("1 selected"))

		dialogs := watchDialogs(page)
		require.NoError(t, selected.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note")).ToHaveCount(1))
		require.Equal(t, "Delete 1 note? This can't be undone.", dialogs.last())

		count--
		left := waitNotes(t, app, user.ID, count)
		require.Equal(t, n.ID, left[0].ID)

		// the page records Opus in WebM, the type it picks first in Chromium
		client := app.Client(t)
		client.LoginAs(user.Email)
		resp := client.Get("/controls/notes/" + n.ID + "/audio").RequireStatus(http.StatusOK)
		require.Equal(t, "audio/webm", resp.Header.Get("Content-Type"))
		require.Contains(t, resp.Body, "A_OPUS")

		openCapture(t, page, "/controls/notes/new")
	})

	// dictateAndEdit records a note and opens its edit page while the
	// recording is transcribed.
	dictateAndEdit := func(t *testing.T) *model.Note {
		t.Helper()

		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
		record(t, page)
		expectLast(t, page, "A recording of 0:0", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]

		_, err := page.Goto("/controls/notes/" + n.ID + "/edit")
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-status")).ToHaveText("Transcribing…"))

		return n
	}

	t.Run("edit page takes the transcript", func(t *testing.T) {
		n := dictateAndEdit(t)

		require.NoError(t, browser.Expect.Locator(page.GetByLabel("Note")).ToHaveValue(captureTranscript, playwright.LocatorAssertionsToHaveValueOptions{Timeout: new(30000.0)}))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-status")).ToHaveText(""))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-transcript")).ToHaveCount(0))
		require.Equal(t, captureTranscript, noteIn(t, app, n.ID).Body)

		// the text box that came in still edits and saves
		require.NoError(t, page.GetByLabel("Note").Fill("Edited after the transcript"))
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: new(true)}).Click())
		require.Eventually(t, func() bool { return noteIn(t, app, n.ID).Body == "Edited after the transcript" }, 10*time.Second, 100*time.Millisecond)

		openCapture(t, page, "/controls/notes/new")
	})

	t.Run("edit page keeps what was typed", func(t *testing.T) {
		dictateAndEdit(t)

		const mine = "My own words"

		require.NoError(t, page.GetByLabel("Note").Fill(mine))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-transcript")).ToHaveText(captureTranscript, playwright.LocatorAssertionsToHaveTextOptions{Timeout: new(30000.0)}))
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("note-status")).ToContainText("Transcribed while you were writing"))
		require.NoError(t, browser.Expect.Locator(page.GetByLabel("Note")).ToHaveValue(mine))

		openCapture(t, page, "/controls/notes/new")
	})

	photo := widePNG(t)

	t.Run("photo", func(t *testing.T) {
		takePhoto(t, page, "Photo", photo)
		expectLast(t, page, "A photo", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.Len(t, n.Images, 1)
		require.Nil(t, n.AudioUploadID)
		require.True(t, underFocus(n), "tags %v", n.Tags)

		// turned into JPEG and scaled down to NOTE_PHOTO_MAX_DIMENSION, which
		// the largest size the media server makes doesn't scale up
		require.Equal(t, "image/jpeg", n.Images[0].Upload.ContentType)

		client := app.Client(t)
		client.LoginAs(user.Email)
		resp := client.Get("/user-media/" + n.Images[0].Upload.UploadedFname + "/full").RequireStatus(http.StatusOK)

		cfg, _, err := image.DecodeConfig(strings.NewReader(resp.Body))
		require.NoError(t, err)
		require.Equal(t, [2]int{400, 200}, [2]int{cfg.Width, cfg.Height})
	})

	t.Run("photo and dictate", func(t *testing.T) {
		takePhoto(t, page, "Photo + dictate", photo)
		record(t, page)
		expectLast(t, page, "A photo with a voice caption", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.Len(t, n.Images, 1)
		require.NotNil(t, n.AudioUploadID)
		require.True(t, underFocus(n), "tags %v", n.Tags)
		eventuallyNote(t, app, user.ID, func(m *model.Note) bool { return m.ID == n.ID && m.Body == captureTranscript })
	})

	t.Run("type", func(t *testing.T) {
		const text = "Thought from the bench #idea"

		typeNote(t, page, text)
		expectLast(t, page, text, "Saved")

		// stored as typed: no quote, no recording, no photo
		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.Equal(t, text, n.Body)
		require.Nil(t, n.AudioUploadID)
		require.Empty(t, n.Images)
		require.True(t, underFocus(n), "tags %v", n.Tags)
		require.Equal(t, []string{"idea"}, n.TextTags)
	})

	t.Run("start", func(t *testing.T) {
		openCapture(t, page, "/controls/notes/new?start=voice")
		record(t, page)
		expectLast(t, page, "A recording of", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.NotNil(t, n.AudioUploadID)
		require.Empty(t, n.Images)
		require.True(t, underFocus(n), "tags %v", n.Tags)

		// a file chooser opens only on a tap
		openCapture(t, page, "/controls/notes/new?start=photo")
		takePhoto(t, page, "Tap to take a photo", photo)
		expectLast(t, page, "A photo", "Saved")

		count++
		n = waitNotes(t, app, user.ID, count)[0]
		require.Len(t, n.Images, 1)
		require.Nil(t, n.AudioUploadID)
		require.True(t, underFocus(n), "tags %v", n.Tags)
	})

	t.Run("start voice with a tap", func(t *testing.T) {
		// a browser that opens the microphone only after a tap
		phone := phonePage(t, app, user, true)
		require.NoError(t, phone.AddInitScript(playwright.Script{Content: new(micNeedsTap)}))
		openCapture(t, phone, "/controls/notes/new?start=voice")

		tap := phone.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Tap to dictate"})
		require.NoError(t, browser.Expect.Locator(tap).ToBeVisible())
		require.NoError(t, browser.Expect.Locator(phone.GetByTestId("timer")).ToBeHidden())
		require.NoError(t, tap.Click())
		require.NoError(t, browser.Expect.Locator(tap).ToBeHidden())
		record(t, phone)
		expectLast(t, phone, "A recording of", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.NotNil(t, n.AudioUploadID)
		require.True(t, underFocus(n), "tags %v", n.Tags)
	})

	t.Run("stop focus", func(t *testing.T) {
		require.NoError(t, page.GetByTestId("focus").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Stop"}).Click())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))

		u, err := factory.GetUser(context.Background(), app.DB, user.ID)
		require.NoError(t, err)
		require.Nil(t, u.NotesFocusTag)

		// opened again, the page captures without a tag
		_, err = page.Reload()
		require.NoError(t, err)
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))
		typeNote(t, page, "Out of focus")
		expectLast(t, page, "Out of focus", "Saved")

		count++
		n := waitNotes(t, app, user.ID, count)[0]
		require.Equal(t, "Out of focus", n.Body)
		require.Empty(t, n.Tags)
	})
}

// micNeedsTap refuses the microphone until the user has tapped the page, as
// some phone browsers do.
const micNeedsTap = `(() => {
  const devices = navigator.mediaDevices
  const open = devices.getUserMedia.bind(devices)
  devices.getUserMedia = (constraints) => navigator.userActivation.isActive
    ? open(constraints)
    : Promise.reject(new DOMException("a tap is needed first", "NotAllowedError"))
})()`

// wakeLockStub stands in for navigator.wakeLock and counts the locks taken
// and released in window.wakeLocks.
const wakeLockStub = `(() => {
  const log = { requested: 0, released: 0 }
  window.wakeLocks = log
  Object.defineProperty(navigator, "wakeLock", {
    configurable: true,
    value: {
      request: async (type) => {
        log.requested++
        let released = false
        return {
          type,
          get released() { return released },
          release: async () => {
            if (!released) log.released++
            released = true
          },
        }
      },
    },
  })
})()`

// wakeLocks are how many locks the page took and released.
func wakeLocks(t *testing.T, page playwright.Page) [2]int {
	t.Helper()

	got, err := page.Evaluate("[window.wakeLocks.requested, window.wakeLocks.released]")
	require.NoError(t, err)

	pair, ok := got.([]any)
	require.True(t, ok, "%#v", got)

	requested, ok := pair[0].(int)
	require.True(t, ok, "%#v", got)
	released, ok := pair[1].(int)
	require.True(t, ok, "%#v", got)

	return [2]int{requested, released}
}

// A recording stops and saves by itself at NOTE_AUDIO_MAX_DURATION, and the
// screen stays on while it runs.
func TestNotesCapture_TimeLimitAndWakeLock(t *testing.T) {
	t.Parallel()

	app := sttApp(t, e2e.WithEnv("NOTE_AUDIO_MAX_DURATION", "2s"))
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, true)
	require.NoError(t, page.AddInitScript(playwright.Script{Content: new(wakeLockStub)}))
	openCapture(t, page, "/controls/notes/new")

	timer := page.GetByTestId("timer")
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Locator(timer).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(timer).ToContainText("of 0:02"))
	require.Eventually(t, func() bool { return wakeLocks(t, page) == [2]int{1, 0} }, 5*time.Second, 50*time.Millisecond)

	// nobody taps Stop
	require.NoError(t, browser.Expect.Locator(timer).ToBeHidden(playwright.LocatorAssertionsToBeHiddenOptions{Timeout: new(10000.0)}))
	expectLast(t, page, "A recording of 0:0", "Saved")
	require.Equal(t, [2]int{1, 1}, wakeLocks(t, page))

	n := waitNotes(t, app, user.ID, 1)[0]
	require.NotNil(t, n.AudioUploadID)
}

// The service worker is registered by the capture page only, controls only
// the notes pages and keeps only the capture page and its assets.
func TestNotesCapture_ServiceWorker(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, false)

	const scopes = "navigator.serviceWorker.getRegistrations().then((rs) => rs.map((r) => new URL(r.scope).pathname))"

	_, err := page.Goto("/")
	require.NoError(t, err)

	got, err := page.Evaluate(scopes)
	require.NoError(t, err)
	require.Empty(t, got)

	openCapture(t, page, "/controls/notes/new")

	got, err = page.Evaluate(scopes)
	require.NoError(t, err)
	require.Equal(t, []any{"/controls/notes/"}, got)

	const cached = `(async () => {
  const out = {}
  for (const key of await caches.keys()) {
    out[key] = (await (await caches.open(key)).keys()).map((r) => new URL(r.url).pathname)
  }
  return out
})()`

	const loaded = `performance.getEntriesByType("resource")
  .map((e) => new URL(e.name).pathname)
  .filter((p) => /\.(js|css|woff2)$/.test(p) && p !== "/controls/notes/sw.js")`

	var kept []string

	require.Eventually(t, func() bool {
		got, err := page.Evaluate(cached)
		require.NoError(t, err)

		byCache, ok := got.(map[string]any)
		require.True(t, ok, "%#v", got)
		require.Len(t, byCache, 1)

		list, ok := byCache["pcom-notes-capture-1"].([]any)
		require.True(t, ok, "%#v", byCache)

		kept = kept[:0]

		for _, p := range list {
			s, ok := p.(string)
			require.True(t, ok, "%#v", p)

			kept = append(kept, s)
		}

		return slices.Contains(kept, "/controls/notes/new") &&
			slices.ContainsFunc(kept, func(p string) bool { return strings.Contains(p, "/js/capture.") })
	}, 10*time.Second, 100*time.Millisecond)

	got, err = page.Evaluate(loaded)
	require.NoError(t, err)

	list, ok := got.([]any)
	require.True(t, ok, "%#v", got)

	assets := []string{"/controls/notes/new"}

	for _, p := range list {
		s, ok := p.(string)
		require.True(t, ok, "%#v", p)

		assets = append(assets, s)
	}

	for _, p := range kept {
		require.Contains(t, assets, p, "kept but not loaded by the capture page")
		require.NotContains(t, p, "main", "the capture page doesn't load main.js")
	}

	// another page is outside its scope
	_, err = page.Goto("/")
	require.NoError(t, err)

	got, err = page.Evaluate("navigator.serviceWorker.getRegistration().then((r) => (r ? r.scope : ''))")
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = page.Evaluate("navigator.serviceWorker.controller === null")
	require.NoError(t, err)
	require.Equal(t, true, got)
}

func TestNotesCapture_OfflineAndDelete(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := capturePage(t, app, user, "/controls/notes/new")
	dialogs := watchDialogs(page)
	network := page.Context()
	status := page.GetByTestId("capture-status")

	// without a speech-to-text provider nothing is recorded
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate"})).ToHaveCount(0))

	bodies := func() []string {
		var out []string
		for _, n := range userNotes(t, app, user.ID) {
			out = append(out, n.Body)
		}

		return out
	}

	deleteLast := func() {
		t.Helper()
		require.NoError(t, page.GetByTestId("last-capture").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("last-capture")).ToBeHidden())
	}

	require.NoError(t, network.SetOffline(true))

	capturedAt := time.Now()
	typeNote(t, page, "Written offline")
	expectLast(t, page, "Written offline", "Saved on the phone, uploading…")

	// Nothing may change while the phone stays offline, and the upload has to
	// come well after the capture; there is no state to wait for, so it sleeps.
	time.Sleep(3 * time.Second)
	require.NoError(t, browser.Expect.Locator(status).ToHaveText("Saved on the phone, uploading…"))
	require.Empty(t, bodies())

	require.NoError(t, network.SetOffline(false))
	expectLast(t, page, "Written offline", "Saved")
	require.Equal(t, []string{"Written offline"}, bodies())

	// the note keeps the time of the capture, not of the upload
	n := userNotes(t, app, user.ID)[0]
	require.WithinDuration(t, capturedAt, n.CapturedAt, time.Second)
	require.Greater(t, n.CreatedAt.Sub(n.CapturedAt), 2*time.Second)

	// an uploaded capture is deleted with the note's confirmation
	deleteLast()
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.Empty(t, bodies())

	// a queued one leaves the queue and never arrives
	require.NoError(t, network.SetOffline(true))
	typeNote(t, page, "Never mind")
	expectLast(t, page, "Never mind", "Saved on the phone, uploading…")
	dialogs.reset()
	deleteLast()
	require.Empty(t, dialogs.last())

	require.NoError(t, network.SetOffline(false))
	typeNote(t, page, "After the delete")
	expectLast(t, page, "After the delete", "Saved")
	require.Equal(t, []string{"After the delete"}, bodies())

	// the controls page's boosted link loads the page with its own scripts
	_, err := page.Goto("/controls")
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Capture", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes/new$`)))
	typeNote(t, page, "From the controls page")
	expectLast(t, page, "From the controls page", "Saved")
}
