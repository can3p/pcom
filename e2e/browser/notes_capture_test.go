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

// capturePage opens a page at phone size whose microphone is allowed.
func capturePage(t *testing.T, app *e2e.App, user *model.User, path string) playwright.Page {
	t.Helper()

	page := browser.Page(t, app, browser.As(user), browser.Configure(func(o *playwright.BrowserNewContextOptions) {
		o.Viewport = &playwright.Size{Width: 390, Height: 844}
		o.IsMobile = new(true)
		o.HasTouch = new(true)
		o.Permissions = []string{"microphone"}
	}))

	_, err := page.Goto(path)
	require.NoError(t, err)

	// the service worker uploads; the network may only go away once it runs
	_, err = page.Evaluate(waitForWorker)
	require.NoError(t, err)

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

func TestNotesCapture_VoicePhotoAndText(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets(),
		e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", wiremock.Shared(t).Watch(t, "notes-capture")),
		e2e.WithEnv("STT_OPENAI_COMPAT_KEY", captureSTTKey),
		e2e.WithEnv("NOTE_PHOTO_MAX_DIMENSION", "400"))
	user := browser.NewUser(t, app)
	mustNote(t, app, user.ID, factory.WithNoteTags("walk"))

	page := capturePage(t, app, user, "/controls/notes/new")
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))

	_, err := page.Goto("/controls/notes/manage")
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Focus on #walk"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("tag-row")).ToContainText("focus"))

	_, err = page.Goto("/controls/notes/new")
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("#walk"))
	require.NoError(t, browser.Expect.Locator(page.GetByText("sent to the speech-to-text provider")).ToBeVisible())

	underFocus := func(n *model.Note) bool { return slices.Contains(n.Tags, "walk") }

	t.Run("dictate", func(t *testing.T) {
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate", Exact: new(true)}).Click())
		record(t, page)
		expectLast(t, page, "A recording of 0:0", "Saved")

		n := eventuallyNote(t, app, user.ID, func(n *model.Note) bool {
			return n.AudioUploadID != nil && len(n.Images) == 0 && n.Body == captureTranscript
		})
		require.True(t, underFocus(n), "tags %v", n.Tags)
	})

	photo := widePNG(t)

	t.Run("photo", func(t *testing.T) {
		takePhoto(t, page, "Photo", photo)
		expectLast(t, page, "A photo", "Saved")

		n := eventuallyNote(t, app, user.ID, func(n *model.Note) bool { return len(n.Images) == 1 && n.AudioUploadID == nil })
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

		n := eventuallyNote(t, app, user.ID, func(n *model.Note) bool {
			return len(n.Images) == 1 && n.AudioUploadID != nil && n.Body == captureTranscript
		})
		require.True(t, underFocus(n), "tags %v", n.Tags)
	})

	t.Run("type", func(t *testing.T) {
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"}).Click())
		require.NoError(t, page.GetByLabel("Note").Fill("Thought from the bench #idea"))
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save note"}).Click())
		expectLast(t, page, "Thought from the bench #idea", "Saved")

		n := eventuallyNote(t, app, user.ID, func(n *model.Note) bool { return n.Body == "Thought from the bench #idea" })
		require.True(t, underFocus(n), "tags %v", n.Tags)
		require.Equal(t, []string{"idea"}, n.TextTags)
	})

	t.Run("start", func(t *testing.T) {
		_, err := page.Goto("/controls/notes/new?start=voice")
		require.NoError(t, err)
		record(t, page)
		expectLast(t, page, "A recording of", "Saved")

		// a file chooser opens only on a tap
		_, err = page.Goto("/controls/notes/new?start=photo")
		require.NoError(t, err)
		takePhoto(t, page, "Tap to take a photo", photo)
		expectLast(t, page, "A photo", "Saved")
	})

	t.Run("stop focus", func(t *testing.T) {
		require.NoError(t, page.GetByTestId("focus").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Stop"}).Click())
		require.NoError(t, browser.Expect.Locator(page.GetByTestId("focus")).ToContainText("No focus tag"))
	})
}

func TestNotesCapture_OfflineAndDelete(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := capturePage(t, app, user, "/controls/notes/new")
	dialogs := watchDialogs(page)
	network := page.Context()

	// without a speech-to-text provider nothing is recorded
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Dictate"})).ToHaveCount(0))

	typeNote := func(text string) {
		t.Helper()
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"}).Click())
		require.NoError(t, page.GetByLabel("Note").Fill(text))
		require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save note"}).Click())
	}

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
	typeNote("Written offline")
	expectLast(t, page, "Written offline", "Saved on the phone, uploading…")
	require.Empty(t, bodies())

	require.NoError(t, network.SetOffline(false))
	expectLast(t, page, "Written offline", "Saved")
	require.Equal(t, []string{"Written offline"}, bodies())

	// an uploaded capture is deleted with the note's confirmation
	deleteLast()
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	require.Empty(t, bodies())

	// a queued one leaves the queue and never arrives
	require.NoError(t, network.SetOffline(true))
	typeNote("Never mind")
	expectLast(t, page, "Never mind", "Saved on the phone, uploading…")
	dialogs.reset()
	deleteLast()
	require.Empty(t, dialogs.last())

	require.NoError(t, network.SetOffline(false))
	typeNote("After the delete")
	expectLast(t, page, "After the delete", "Saved")
	require.Equal(t, []string{"After the delete"}, bodies())

	// the controls page's boosted link loads the page with its own scripts
	_, err := page.Goto("/controls")
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Capture", Exact: new(true)}).Click())
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes/new$`)))
	typeNote("From the controls page")
	expectLast(t, page, "From the controls page", "Saved")
}
