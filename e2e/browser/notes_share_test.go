//go:build browser

// The share target of the installed app (site.webmanifest): what another app
// shares into pcom becomes a note. The tests post the share sheet's form from
// the page, as the phone does: through the capture page's service worker,
// online and offline, and to the server when no worker is there yet; and the
// manifest that declares the share target.
package browser_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

// sharedFile is a file the share sheet sends in the field of its kind.
type sharedFile struct {
	field string
	file  playwright.InputFile
}

// shareSheet posts a share to the share target from the page, as a
// navigation with the fields of site.webmanifest, and waits for the capture
// page it ends on.
func shareSheet(t *testing.T, page playwright.Page, fields map[string]string, files ...sharedFile) {
	t.Helper()

	// Evaluate serializes only untyped maps and slices
	values := map[string]any{}
	for k, v := range fields {
		values[k] = v
	}

	names := []any{}
	for _, f := range files {
		names = append(names, f.field)
	}

	_, err := page.Evaluate(`([fields, files]) => {
  document.getElementById("share-sheet")?.remove()
  const form = document.createElement("form")
  form.id = "share-sheet"
  form.method = "POST"
  form.action = "/controls/notes/share"
  form.enctype = "multipart/form-data"
  form.hidden = true
  for (const [name, value] of Object.entries(fields || {})) {
    const t = document.createElement("textarea")
    t.name = name
    t.value = value
    form.append(t)
  }
  (files || []).forEach((name, i) => {
    const input = document.createElement("input")
    input.type = "file"
    input.name = name
    input.dataset.file = String(i)
    form.append(input)
  })
  document.body.append(form)
}`, []any{values, names})
	require.NoError(t, err)

	for i, f := range files {
		require.NoError(t, page.Locator(`#share-sheet input[data-file="`+strconv.Itoa(i)+`"]`).SetInputFiles([]playwright.InputFile{f.file}))
	}

	_, err = page.Evaluate(`setTimeout(() => document.getElementById("share-sheet").submit())`)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.Locator("#share-sheet")).ToHaveCount(0))
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile(`/controls/notes/new$`)))
}

func TestNotesShare_ThroughTheServiceWorker(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, false)
	openCapture(t, page, "/controls/notes/new")

	passage := "“It is a narrow mind which cannot look at a subject from various points of view.”\nMiddlemarch by George Eliot"
	ogg := append([]byte("OggS"), make([]byte, 256)...)

	for _, r := range []struct {
		name   string
		fields map[string]string
		files  []sharedFile
		card   string
		check  func(t *testing.T, n *model.Note)
	}{
		{"plain text", map[string]string{"text": "Buy oat milk"}, nil, "Buy oat milk",
			func(t *testing.T, n *model.Note) { require.Equal(t, "> Buy oat milk", n.Body) }},
		{"a passage", map[string]string{"title": "Middlemarch", "text": passage}, nil, "“It is a narrow mind",
			func(t *testing.T, n *model.Note) {
				require.Equal(t, "> “It is a narrow mind which cannot look at a subject from various points of view.”\n> Middlemarch by George Eliot", n.Body)
			}},
		{"a lone link", map[string]string{"title": "", "text": "", "url": "https://example.com/article"}, nil, "https://example.com/article",
			func(t *testing.T, n *model.Note) { require.Equal(t, "https://example.com/article", n.Body) }},
		{"an image", nil, []sharedFile{{"image", playwright.InputFile{Name: "lens.png", MimeType: "image/png", Buffer: widePNG(t)}}}, "A photo",
			func(t *testing.T, n *model.Note) {
				require.Empty(t, n.Body)
				require.Len(t, n.Images, 1)
			}},
		// a recording the page can't decode shows no length, not 0:00
		{"audio", nil, []sharedFile{{"audio", playwright.InputFile{Name: "memo.ogg", MimeType: "audio/ogg", Buffer: ogg}}}, "A recording",
			func(t *testing.T, n *model.Note) { require.NotNil(t, n.AudioUploadID) }},
	} {
		t.Run(r.name, func(t *testing.T) {
			before := len(userNotes(t, app, user.ID))

			shareSheet(t, page, r.fields, r.files...)
			expectLast(t, page, r.card, "Saved")
			require.NoError(t, browser.Expect.Locator(page.GetByTestId("last-capture")).Not().ToContainText("0:00"))
			r.check(t, waitNotes(t, app, user.ID, before+1)[0])
		})
	}

	t.Run("offline", func(t *testing.T) {
		before := len(userNotes(t, app, user.ID))

		require.NoError(t, page.Context().SetOffline(true))
		shareSheet(t, page, map[string]string{"text": "Shared on the train"})
		expectLast(t, page, "Shared on the train", "Saved on the phone, uploading…")
		require.Len(t, userNotes(t, app, user.ID), before)

		require.NoError(t, page.Context().SetOffline(false))
		expectLast(t, page, "Shared on the train", "Saved")
		require.Equal(t, "> Shared on the train", waitNotes(t, app, user.ID, before+1)[0].Body)
	})
}

// Before the capture page has ever run, no service worker takes the share:
// the server saves it and sends the phone to the capture page, which shows
// the note as the last capture, with its Delete. The page shows only the
// viewer's own note there.
func TestNotesShare_WithoutAServiceWorker(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, false)
	dialogs := watchDialogs(page)

	_, err := page.Goto("/controls/notes")
	require.NoError(t, err)

	shareSheet(t, page, map[string]string{"title": "Article", "text": "", "url": "https://example.com/a"})
	expectLast(t, page, "Article", "Saved")

	notes := waitNotes(t, app, user.ID, 1)
	require.Equal(t, "> Article\n> https://example.com/a", notes[0].Body)

	// a reload doesn't bring the card back once the note is deleted
	require.NoError(t, page.GetByTestId("last-capture").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("last-capture")).ToBeHidden())
	require.Equal(t, "Delete this note? This can't be undone.", dialogs.last())
	waitNotes(t, app, user.ID, 0)

	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(page.GetByTestId("last-capture")).ToBeHidden())

	// another user's note is not shown
	other := browser.NewUser(t, app)
	theirs := mustNote(t, app, other.ID)
	fresh := phonePage(t, app, user, false)

	_, err = fresh.Goto("/controls/notes/new?shared=" + theirs.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(fresh.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(fresh.GetByTestId("last-capture")).ToBeHidden())
}

// The pages link the manifest at its fixed URL on the app's origin, and the
// icons it names are served.
func TestNotesShare_Manifest(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	client := app.Client(t)

	href, ok := client.Get("/").RequireStatus(http.StatusOK).Doc().Find(`link[rel="manifest"]`).Attr("href")
	require.True(t, ok)
	require.Equal(t, "/site.webmanifest", href)

	resp := client.Get(href).RequireStatus(http.StatusOK)
	require.Equal(t, "application/manifest+json", resp.Header.Get("Content-Type"))

	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &m))
	require.Len(t, m.Icons, 2)

	for _, i := range m.Icons {
		icon := client.Get(i.Src).RequireStatus(http.StatusOK)
		require.Equal(t, "image/png", icon.Header.Get("Content-Type"))
	}
}
