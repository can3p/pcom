//go:build browser

// The share target of the installed app (site.webmanifest): what another app
// shares into pcom becomes a note. The tests post the share sheet's form from
// the page, as the phone does. Online a share goes through the capture page's
// service worker to the server's route; offline the worker queues it, within
// the server's caps, and uploads it when the network is back; with no worker
// yet the server takes it as well. A share posted from another site creates
// nothing. The manifest declares the share target and the app shortcuts.
package browser_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
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

// queueRecord is a record of the capture page's IndexedDB queue, as far as
// the tests look at it.
type queueRecord struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind"`
	Status  string  `json:"status"`
	Text    string  `json:"text"`
	Error   string  `json:"error"`
	Files   int     `json:"files"`
	SavedAt float64 `json:"savedAt"`
}

// readQueue reads the queue of the page's origin, oldest first.
const readQueue = `new Promise((resolve, reject) => {
  const req = indexedDB.open("pcom-notes", 1)
  req.onupgradeneeded = () => req.result.createObjectStore("captures", { keyPath: "id" })
  req.onerror = () => reject(req.error)
  req.onsuccess = () => {
    const db = req.result
    const get = db.transaction("captures").objectStore("captures").getAll()
    get.onerror = () => reject(get.error)
    get.onsuccess = () => {
      db.close()
      resolve(JSON.stringify(get.result
        .sort((a, b) => a.savedAt - b.savedAt)
        .map((r) => ({
          id: r.id,
          kind: r.kind || "",
          status: r.status,
          text: r.text || "",
          error: r.error || "",
          files: (r.images || []).length + (r.audio ? 1 : 0),
          savedAt: r.savedAt,
        }))))
    }
  }
})`

func queue(t *testing.T, page playwright.Page) []queueRecord {
	t.Helper()

	got, err := page.Evaluate(readQueue)
	require.NoError(t, err)

	s, ok := got.(string)
	require.True(t, ok, "%#v", got)

	var out []queueRecord
	require.NoError(t, json.Unmarshal([]byte(s), &out))

	return out
}

// workerShares are the records of the queue the service worker made from a
// share.
func workerShares(t *testing.T, page playwright.Page) []queueRecord {
	t.Helper()

	var out []queueRecord

	for _, r := range queue(t, page) {
		if r.Kind == "share" {
			out = append(out, r)
		}
	}

	return out
}

// waitForCachedPage resolves once the service worker keeps the capture page,
// which it answers a navigation with offline, and the scripts, styles and
// fonts the page loaded.
const waitForCachedPage = `(async () => {
  await document.fonts.ready
  const kept = (keys) => keys.map((r) => new URL(r.url).pathname)
  const assets = performance.getEntriesByType("resource")
    .map((e) => new URL(e.name).pathname)
    .filter((p) => p.startsWith("/static/") && /\.(js|css|woff2)$/.test(p))
  for (let i = 0; i < 100; i++) {
    const cache = await caches.open("pcom-notes-capture-1")
    const paths = kept(await cache.keys())
    if (paths.includes("/controls/notes/new") && assets.every((s) => paths.includes(s))) return true
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error("the capture page is not kept")
})()`

// offlineReady opens the capture page, so its service worker takes the
// shares and keeps the page for offline use.
func offlineReady(t *testing.T, page playwright.Page) {
	t.Helper()

	openCapture(t, page, "/controls/notes/new")

	_, err := page.Evaluate(waitForCachedPage)
	require.NoError(t, err)
}

// offlinePhonePage is phonePage for a test that takes the network away: the
// capture page the worker answers offline still asks for its icons, which
// the worker doesn't keep.
func offlinePhonePage(t *testing.T, app *e2e.App, user *model.User) playwright.Page {
	t.Helper()

	return browser.Page(t, app, browser.As(user),
		browser.Allow(`request failed: GET \S+/(favicon|android-chrome)-[^ ]*: net::ERR_INTERNET_DISCONNECTED`),
		browser.Allow(`console.error: Failed to load resource: net::ERR_INTERNET_DISCONNECTED`),
		browser.Configure(func(o *playwright.BrowserNewContextOptions) {
			o.Viewport = &playwright.Size{Width: 390, Height: 844}
			o.IsMobile = new(true)
			o.HasTouch = new(true)
		}))
}

// sharePattern is the share target's URL, for routing the worker's requests.
const sharePattern = "**/controls/notes/share"

// goOffline takes the network away. The share the worker sends on fails as
// well by a route of its own: Chromium may apply the offline state to a
// running service worker a little later than to the page.
func goOffline(t *testing.T, page playwright.Page) {
	t.Helper()

	require.NoError(t, page.Context().Route(sharePattern, func(r playwright.Route) { _ = r.Abort("internetdisconnected") }))
	require.NoError(t, page.Context().SetOffline(true))
}

func goOnline(t *testing.T, page playwright.Page) {
	t.Helper()

	require.NoError(t, page.Context().Unroute(sharePattern))
	require.NoError(t, page.Context().SetOffline(false))
}

// shareKind is a share of each kind the share sheet sends: the card the
// capture page shows for it and the note it becomes.
type shareKind struct {
	name   string
	fields map[string]string
	files  []sharedFile
	card   string
	check  func(t *testing.T, n *model.Note)
}

func shareKinds(t *testing.T) []shareKind {
	t.Helper()

	passage := "“It is a narrow mind which cannot look at a subject from various points of view.”\nMiddlemarch by George Eliot"
	ogg := append([]byte("OggS"), make([]byte, 256)...)

	return []shareKind{
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
		{"audio", nil, []sharedFile{{"audio", playwright.InputFile{Name: "memo.ogg", MimeType: "audio/ogg", Buffer: ogg}}}, "A recording",
			func(t *testing.T, n *model.Note) { require.NotNil(t, n.AudioUploadID) }},
	}
}

// Online, the service worker sends a share to the server, which saves it:
// nothing goes into the worker's queue, and the page shows the server's note.
func TestNotesShare_OnlineThroughTheServer(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, false)
	offlineReady(t, page)

	for _, r := range shareKinds(t) {
		t.Run(r.name, func(t *testing.T) {
			before := len(userNotes(t, app, user.ID))

			shareSheet(t, page, r.fields, r.files...)
			expectLast(t, page, r.card, "Saved")
			// a recording the page can't decode shows no length, not 0:00
			require.NoError(t, browser.Expect.Locator(page.GetByTestId("last-capture")).Not().ToContainText("0:00"))

			n := waitNotes(t, app, user.ID, before+1)[0]
			r.check(t, n)

			require.Empty(t, workerShares(t, page), "the worker queued an online share")
			require.True(t, slices.ContainsFunc(queue(t, page), func(q queueRecord) bool { return q.ID == "shared-"+n.ID }),
				"the card is not the server's note")
		})
	}
}

// Offline, the service worker queues a share and the page shows it as kept
// on the phone; it is uploaded when the network is back.
func TestNotesShare_OfflineThroughTheWorker(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := offlinePhonePage(t, app, user)
	offlineReady(t, page)

	for _, r := range shareKinds(t) {
		t.Run(r.name, func(t *testing.T) {
			before := len(userNotes(t, app, user.ID))
			queued := len(workerShares(t, page))

			goOffline(t, page)
			shareSheet(t, page, r.fields, r.files...)
			expectLast(t, page, r.card, "Saved on the phone, uploading…")

			shares := workerShares(t, page)
			require.Len(t, shares, queued+1)
			require.Equal(t, "queued", shares[len(shares)-1].Status)
			require.Equal(t, len(r.files), shares[len(shares)-1].Files)
			require.Len(t, userNotes(t, app, user.ID), before)

			goOnline(t, page)
			expectLast(t, page, r.card, "Saved")
			r.check(t, waitNotes(t, app, user.ID, before+1)[0])
		})
	}
}

// Offline, a share over the server's caps is not queued, and the page says
// why.
func TestNotesShare_OfflineOverTheCaps(t *testing.T) {
	t.Parallel()

	const capBytes = 4096

	app := e2e.Start(t, e2e.WithRealAssets(),
		e2e.WithEnv("NOTE_AUDIO_MAX_BYTES", strconv.Itoa(capBytes)),
		e2e.WithEnv("MAX_UPLOAD_SIZE", strconv.Itoa(capBytes)))
	user := browser.NewUser(t, app)
	page := offlinePhonePage(t, app, user)
	offlineReady(t, page)

	big := make([]byte, capBytes+1)

	for _, r := range []struct {
		name, card, status string
		file               sharedFile
	}{
		{"a recording", "A recording", "Not saved: The recording is larger than 4 KiB.",
			sharedFile{"audio", playwright.InputFile{Name: "memo.ogg", MimeType: "audio/ogg", Buffer: append([]byte("OggS"), big...)}}},
		{"a photo", "A photo", "Not saved: A photo can be at most 4 KiB.",
			sharedFile{"image", playwright.InputFile{Name: "lens.png", MimeType: "image/png", Buffer: big}}},
	} {
		t.Run(r.name, func(t *testing.T) {
			goOffline(t, page)
			shareSheet(t, page, nil, r.file)
			expectLast(t, page, r.card, r.status)

			for _, q := range workerShares(t, page) {
				require.NotEqual(t, "queued", q.Status)
				require.Zero(t, q.Files, "a refused share keeps its files")
			}

			goOnline(t, page)
		})
	}

	require.Empty(t, userNotes(t, app, user.ID))
}

// shareTextCase is a row of pkg/web/app/testdata/share_text.json, the table
// of TestShareText.
type shareTextCase struct {
	Name, Title, Text, URL, Want string
}

// The service worker joins the parts of a share as the server's shareText
// does: TestShareText's table, run through the worker offline.
func TestNotesShare_WorkerShareTextMatchesServer(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("../../pkg/web/app/testdata/share_text.json")
	require.NoError(t, err)

	var cases []shareTextCase
	require.NoError(t, json.Unmarshal(src, &cases))
	require.NotEmpty(t, cases)

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := offlinePhonePage(t, app, user)
	offlineReady(t, page)
	goOffline(t, page)

	for _, r := range cases {
		t.Run(r.Name, func(t *testing.T) {
			before := workerShares(t, page)

			shareSheet(t, page, map[string]string{"title": r.Title, "text": r.Text, "url": r.URL})

			after := workerShares(t, page)
			if r.Want == "" {
				require.Len(t, after, len(before), "an empty share is queued")
				return
			}

			require.Len(t, after, len(before)+1)
			require.Equal(t, r.Want, after[len(after)-1].Text)
		})
	}
}

// A share another site posts while the user is logged in creates nothing:
// the worker sends it to the server, which gets no session cookie with a
// cross-site POST, and queues nothing.
func TestNotesShare_CrossSiteCreatesNothing(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	user := browser.NewUser(t, app)
	page := phonePage(t, app, user, false)

	// the session cookie as the app sets it: Playwright adds cookies without
	// a SameSite, which Chromium would treat as Lax only after two minutes
	cookies, err := page.Context().Cookies(app.URL)
	require.NoError(t, err)
	require.NotEmpty(t, cookies)

	var lax []playwright.OptionalCookie
	for _, c := range cookies {
		lax = append(lax, playwright.OptionalCookie{
			Name: c.Name, Value: c.Value, URL: new(app.URL),
			HttpOnly: new(c.HttpOnly), SameSite: playwright.SameSiteAttributeLax,
		})
	}

	require.NoError(t, page.Context().ClearCookies())
	require.NoError(t, page.Context().AddCookies(lax))

	offlineReady(t, page)

	// another site, on another host name, posts a share as soon as it opens
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><title>Win a prize</title>
<form id="f" method="POST" action="%s/controls/notes/share" enctype="multipart/form-data">
<input name="text" value="A forged share">
</form>
<script>document.getElementById("f").submit()</script>`, app.URL)
	}))
	t.Cleanup(evil.Close)

	other, err := url.Parse(evil.URL)
	require.NoError(t, err)
	other.Host = "localhost:" + other.Port()
	require.NotEqual(t, other.Hostname(), mustHost(t, app.URL))

	_, err = page.Goto(other.String())
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Page(page).ToHaveURL(regexp.MustCompile("^"+regexp.QuoteMeta(app.URL)+"/")))
	require.NoError(t, page.WaitForLoadState())
	require.Contains(t, page.URL(), "/login")

	// the page is on the app's origin, whose queue a share would be in, and
	// a queued share would be uploaded once the capture page gives it the
	// session's token
	require.Empty(t, workerShares(t, page), "the worker queued a cross-site share")
	require.Empty(t, userNotes(t, app, user.ID))
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	require.NoError(t, err)

	return u.Hostname()
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

	// another user's note is not shown: the server's page doesn't name it,
	// while it names the viewer's own
	other := browser.NewUser(t, app)
	theirs := mustNote(t, app, other.ID)
	mine := mustNote(t, app, user.ID)
	client := app.Client(t)
	client.LoginAs(user.Email)

	sharedValue := func(id string) (string, bool) {
		return client.Get("/controls/notes/new?shared=" + id).RequireStatus(http.StatusOK).Doc().
			Find("main.capture").Attr("data-notes-capture-shared-value")
	}

	_, ok := sharedValue(theirs.ID)
	require.False(t, ok, "the page names another user's note")

	own, ok := sharedValue(mine.ID)
	require.True(t, ok)
	require.Contains(t, own, mine.ID)

	fresh := phonePage(t, app, user, false)

	_, err = fresh.Goto("/controls/notes/new?shared=" + theirs.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(fresh.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Type"})).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(fresh.GetByTestId("last-capture")).ToBeHidden())
}

// manifestOf reads the served manifest.
func manifestOf(t *testing.T, client *e2e.Client) (href string, m struct {
	Icons []struct {
		Src string `json:"src"`
	} `json:"icons"`
	Shortcuts []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"shortcuts"`
},
) {
	t.Helper()

	href, ok := client.Get("/").RequireStatus(http.StatusOK).Doc().Find(`link[rel="manifest"]`).Attr("href")
	require.True(t, ok)

	resp := client.Get(href).RequireStatus(http.StatusOK)
	require.Equal(t, "application/manifest+json", resp.Header.Get("Content-Type"))
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &m))

	return href, m
}

// The pages link the manifest at its fixed URL on the app's origin, and the
// icons it names are served.
func TestNotesShare_Manifest(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	client := app.Client(t)

	href, m := manifestOf(t, client)
	require.Equal(t, "/site.webmanifest", href)
	require.Len(t, m.Icons, 2)

	for _, i := range m.Icons {
		icon := client.Get(i.Src).RequireStatus(http.StatusOK)
		require.Equal(t, "image/png", icon.Header.Get("Content-Type"))
	}
}

// The app shortcuts of the manifest open the capture page dictating, and
// ready to take a photo on a tap.
func TestNotesShare_Shortcuts(t *testing.T) {
	t.Parallel()

	app := sttApp(t)
	user := browser.NewUser(t, app)
	_, m := manifestOf(t, app.Client(t))

	shortcuts := map[string]string{}
	for _, s := range m.Shortcuts {
		shortcuts[s.Name] = s.URL
	}

	require.Equal(t, []string{"Dictate", "Photo"}, slices.Sorted(maps.Keys(shortcuts)))

	t.Run("Dictate", func(t *testing.T) {
		require.True(t, strings.HasSuffix(shortcuts["Dictate"], "?start=voice"), shortcuts["Dictate"])

		page := phonePage(t, app, user, true)
		openCapture(t, page, shortcuts["Dictate"])
		record(t, page)
		expectLast(t, page, "A recording of", "Saved")
		require.NotNil(t, waitNotes(t, app, user.ID, 1)[0].AudioUploadID)
	})

	t.Run("Photo", func(t *testing.T) {
		require.True(t, strings.HasSuffix(shortcuts["Photo"], "?start=photo"), shortcuts["Photo"])

		page := phonePage(t, app, user, false)
		openCapture(t, page, shortcuts["Photo"])
		takePhoto(t, page, "Tap to take a photo", widePNG(t))
		expectLast(t, page, "A photo", "Saved")
		require.Len(t, waitNotes(t, app, user.ID, 2)[0].Images, 1)
	})
}
