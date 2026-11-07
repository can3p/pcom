package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/web"
	"github.com/stretchr/testify/require"
)

// apiRoute is a route of the API and whether a notes key opens it.
type apiRoute struct {
	method, path string
	notes        bool
}

// apiRoutes are the API's routes, read from their source as
// TestGuards_RouteTableMatchesSource reads the others, so a new route is
// covered without a new row: api.go mounts the full-key group, api_notes.go
// the notes group. Path parameters get a uuid.
func apiRoutes(t *testing.T) []apiRoute {
	t.Helper()

	var routes []apiRoute

	for file, notes := range map[string]bool{"../pkg/web/app/api.go": false, "../pkg/web/app/api_notes.go": true} {
		src, err := os.ReadFile(file)
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(src), "\n") {
			if m := routeRe.FindStringSubmatch(line); m != nil {
				path := "/api/v1" + strings.ReplaceAll(m[3], ":id", "0190a5c4-7b1e-7d2a-9a43-1c2b3d4e5f60")
				routes = append(routes, apiRoute{m[2], path, notes})
			}
		}
	}

	require.NotEmpty(t, routes)

	return routes
}

func notesScope(k *model.UserAPIKey) { k.Scope = model.APIKeyScopeNotes }

// apiCall sends an API request with the Authorization header auth ("" for none).
func apiCall(t *testing.T, app *e2e.App, method, path, auth string) *e2e.Response {
	t.Helper()

	req, err := http.NewRequest(method, app.URL+path, strings.NewReader("{}"))
	require.NoError(t, err)

	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	return app.Client(t).Do(req)
}

// TestAPIKeys_EveryRouteByKeyState: without a valid key every API route
// answers 401, a notes key gets 403 on every route outside the notes API,
// and a full key passes the key check on all of them.
func TestAPIKeys_EveryRouteByKeyState(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user, owner := newLoggedIn(t, app)

	full := testutil.Must(factory.APIKey(ctx, app.DB, user.ID))(t)
	notes := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t)
	revoked := testutil.Must(factory.APIKey(ctx, app.DB, user.ID))(t)
	owner.PostJSON("/controls/action/revoke_api_key", map[string]string{"id": revoked.ID}).RequireStatus(http.StatusOK)

	states := []struct {
		name string
		auth string
		// want is the status; 0 is any but 401 and 403.
		want, wantNotes int
	}{
		{"no key", "", http.StatusUnauthorized, http.StatusUnauthorized},
		{"malformed header", "Token " + full.APIKey, http.StatusUnauthorized, http.StatusUnauthorized},
		{"unknown key", "Bearer not-a-key", http.StatusUnauthorized, http.StatusUnauthorized},
		{"revoked key", "Bearer " + revoked.APIKey, http.StatusUnauthorized, http.StatusUnauthorized},
		{"notes key", "Bearer " + notes.APIKey, http.StatusForbidden, 0},
		{"full key", "Bearer " + full.APIKey, 0, 0},
	}

	for _, r := range apiRoutes(t) {
		for _, s := range states {
			t.Run(r.method+" "+r.path+"/"+s.name, func(t *testing.T) {
				t.Parallel()

				want := s.want
				if r.notes {
					want = s.wantNotes
				}

				resp := apiCall(t, app, r.method, r.path, s.auth)

				if want == 0 {
					require.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode, resp.Body)
					return
				}

				resp.RequireStatus(want)

				if want == http.StatusUnauthorized {
					require.Equal(t, "Bearer", resp.Header.Get("WWW-Authenticate"))
				}
			})
		}
	}
}

// TestAPIKeys_SettingsActions: a key is created with a name and a scope and
// shown once, in the response that created it; its use is recorded; it can
// be revoked by its owner only, and then stops working.
func TestAPIKeys_SettingsActions(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user, owner := newLoggedIn(t, app)
	_, stranger := newLoggedIn(t, app)

	created := owner.PostJSON("/controls/action/generate_api_key", map[string]string{"name": "iPhone", "scope": "notes"}).
		RequireStatus(http.StatusOK)
	plain := created.Doc().Find(`[aria-label="Your new API key"]`).Text()
	require.NotEmpty(t, plain)

	keys := testutil.Must(factory.ListAPIKeys(ctx, app.DB, user.ID))(t)
	require.Len(t, keys, 1)
	require.Equal(t, "iPhone", keys[0].Name)
	require.Equal(t, model.APIKeyScopeNotes, keys[0].Scope)
	require.Equal(t, model.HashAPIKey(plain), keys[0].KeyHash)
	require.Nil(t, keys[0].LastUsedAt)

	apiCall(t, app, http.MethodGet, "/api/v1/notes", "Bearer "+plain).RequireStatus(http.StatusOK)
	keys = testutil.Must(factory.ListAPIKeys(ctx, app.DB, user.ID))(t)
	require.NotNil(t, keys[0].LastUsedAt, "the use is recorded")
	require.WithinDuration(t, time.Now(), *keys[0].LastUsedAt, time.Minute)

	require.NotContains(t, owner.Get("/controls/settings").RequireStatus(http.StatusOK).Body, plain, "the settings page never shows a key")

	bad := owner.PostJSON("/controls/action/generate_api_key", map[string]string{"scope": "admin"}).RequireStatus(http.StatusOK)
	require.Contains(t, bad.Doc().Find(".field-error").Text(), "Choose what the key may do.")
	require.Empty(t, bad.Doc().Find(`[aria-label="Your new API key"]`).Nodes)

	stranger.PostJSON("/controls/action/revoke_api_key", map[string]string{"id": keys[0].ID}).RequireStatus(http.StatusBadRequest)
	require.Len(t, testutil.Must(factory.ListAPIKeys(ctx, app.DB, user.ID))(t), 1, "another user's key stays")

	revoked := owner.PostJSON("/controls/action/revoke_api_key", map[string]string{"id": keys[0].ID}).RequireStatus(http.StatusOK)
	require.NotContains(t, revoked.Body, plain)
	require.Empty(t, testutil.Must(factory.ListAPIKeys(ctx, app.DB, user.ID))(t))
	apiCall(t, app, http.MethodGet, "/api/v1/notes", "Bearer "+plain).RequireStatus(http.StatusUnauthorized)
}

// part is a field or a file of a multipart form.
type part struct {
	name, value     string
	filename, ctype string
	data            []byte
}

func field(name, value string) part { return part{name: name, value: value} }

func file(name, filename, ctype string, data []byte) part {
	return part{name: name, filename: filename, ctype: ctype, data: data}
}

// postNote sends POST /api/v1/notes with the key and the parts as
// multipart/form-data, the way Shortcuts' "Get Contents of URL" with a Form
// body sends them.
func postNote(t *testing.T, app *e2e.App, key string, parts ...part) *e2e.Response {
	t.Helper()

	var body bytes.Buffer

	mw := multipart.NewWriter(&body)

	for _, p := range parts {
		if p.filename == "" {
			require.NoError(t, mw.WriteField(p.name, p.value))
			continue
		}

		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, p.name, p.filename))
		h.Set("Content-Type", p.ctype)
		w, err := mw.CreatePart(h)
		require.NoError(t, err)
		_, err = w.Write(p.data)
		require.NoError(t, err)
	}

	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, app.URL+"/api/v1/notes", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+key)

	return app.Client(t).Do(req)
}

// apiError is the notes API's answer to refused input.
type apiError struct {
	Errors []string `json:"errors"`
	Field  string   `json:"field"`
}

func smallPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))))

	return buf.Bytes()
}

func smallJPEG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil))

	return buf.Bytes()
}

// Recordings and photos as phones make them.
const (
	m4aRecording = "\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00isomM4A mp42 and the rest of a memo"
	heicPhoto    = "\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic and the rest of a photo"
)

// notesApp starts the app the notes API tests share: transcription on,
// against a backend nobody answers at, so a recording's note stays pending,
// and small caps, so a test can pass them.
func notesApp(t *testing.T) *e2e.App {
	t.Helper()

	return e2e.Start(t,
		e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", "http://127.0.0.1:9/stt"),
		e2e.WithEnv("STT_OPENAI_COMPAT_KEY", "unused"),
		e2e.WithEnv("NOTE_AUDIO_MAX_BYTES", "1024"),
		e2e.WithEnv("NOTE_CAPTURE_MAX_BYTES", "65536"),
		e2e.WithEnv("MAX_BODY_SIZE", "65536"),
	)
}

// TestAPINotes_Create: every field of POST /api/v1/notes, alone and
// together, and the refusals, each with the field to fix.
func TestAPINotes_Create(t *testing.T) {
	t.Parallel()

	app := notesApp(t)
	ctx := context.Background()
	png1, jpg := smallPNG(t), smallJPEG(t)
	at := "2026-11-07T09:30:00+01:00"
	long := strings.Repeat("x", 301)

	cases := []struct {
		name string
		// before are requests sent first with the same key.
		before [][]part
		// focus is set through the notes page first.
		focus     string
		parts     []part
		wantField string
		wantMsg   string
		want      func(t *testing.T, n web.ApiNote)
	}{
		{name: "text only", parts: []part{field("text", " hello ")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "hello", n.Body)
			require.Empty(t, n.Tags)
			require.Empty(t, n.Source)
			require.Empty(t, n.Images)
			require.Equal(t, model.NoteTranscriptionNone, n.TranscriptionStatus)
			require.WithinDuration(t, time.Now(), n.CapturedAt, time.Minute, "no captured_at is now")
		}},
		{name: "kind note", parts: []part{field("text", "a thought"), field("kind", "note")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "a thought", n.Body)
		}},
		{name: "kind share", parts: []part{field("text", "line one\nline two"), field("kind", "share")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "> line one\n> line two", n.Body)
			require.Empty(t, n.Source)
		}},
		{name: "kind quote", parts: []part{field("text", "a passage"), field("kind", "quote"), field("source", "Dune")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "> a passage", n.Body)
			require.Equal(t, "Dune", n.Source)
		}},
		{name: "quote without source takes the previous quote's", before: [][]part{{field("text", "first"), field("kind", "quote"), field("source", "Dune")}},
			parts: []part{field("text", "second"), field("kind", "quote")}, want: func(t *testing.T, n web.ApiNote) {
				require.Equal(t, "Dune", n.Source)
			}},
		{name: "lone URL stays bare", parts: []part{field("text", " https://example.test/page "), field("kind", "share")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "https://example.test/page", n.Body)
		}},
		{name: "unknown kind", parts: []part{field("text", "x"), field("kind", "poem")}, wantField: "kind", wantMsg: "kind is note, quote or share."},
		{name: "tags repeated and comma-separated, normalized", parts: []part{field("text", "x #Inline"), field("tags", "Walk, #Idea"), field("tags", "book")},
			want: func(t *testing.T, n web.ApiNote) {
				require.ElementsMatch(t, []string{"walk", "idea", "book", "inline"}, n.Tags)
			}},
		{name: "too many tags", parts: []part{field("text", "x"), field("tags", "a,b,c,d,e,f,g,h,i,j,k")}, wantField: "tags", wantMsg: "A note can have at most 10 tags."},
		{name: "tag too long", parts: []part{field("text", "x"), field("tags", strings.Repeat("t", 41))}, wantField: "tags", wantMsg: "A tag can be at most 40 characters long."},
		{name: "captured_at", parts: []part{field("text", "x"), field("captured_at", at)}, want: func(t *testing.T, n web.ApiNote) {
			want, err := time.Parse(time.RFC3339, at)
			require.NoError(t, err)
			require.True(t, want.Equal(n.CapturedAt), "%s", n.CapturedAt)
		}},
		{name: "captured_at invalid", parts: []part{field("text", "x"), field("captured_at", "yesterday")}, wantField: "captured_at",
			wantMsg: "captured_at is not an RFC 3339 time, such as 2026-11-07T09:30:00+01:00."},
		{name: "source new", parts: []part{field("text", "x"), field("source", "The Book")}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "The Book", n.Source)
		}},
		{name: "source existing is reused", before: [][]part{{field("text", "x"), field("source", "The Book")}},
			parts: []part{field("text", "y"), field("source", " the  book ")}, want: func(t *testing.T, n web.ApiNote) {
				require.Equal(t, "The Book", n.Source, "found by title whatever the case and spacing")
			}},
		{name: "source too long", parts: []part{field("text", "x"), field("source", long)}, wantField: "title", wantMsg: "A title can be at most 300 characters long."},
		{name: "several images", parts: []part{file("image", "a.png", "image/png", png1), file("image", "b.jpg", "image/jpeg", jpg)},
			want: func(t *testing.T, n web.ApiNote) {
				require.Empty(t, n.Body)
				require.Len(t, n.Images, 2)

				for _, u := range n.Images {
					require.True(t, strings.HasPrefix(u, app.URL+"/user-media/"), u)
				}

				require.True(t, strings.HasSuffix(n.Images[0], ".png"), "in the order sent")
				require.True(t, strings.HasSuffix(n.Images[1], ".jpg"))
			}},
		{name: "a non-image image", parts: []part{file("image", "a.png", "image/png", png1), file("image", "notes.txt", "image/png", []byte("plain text"))},
			wantField: "image", wantMsg: "pcom keeps photos in JPEG, PNG or WebP."},
		{name: "HEIC", parts: []part{file("image", "IMG_0001.HEIC", "image/heic", []byte(heicPhoto))}, wantField: "image",
			wantMsg: "pcom can't read HEIC photos: add \"Convert to JPEG\" before sending the photo in the Shortcut."},
		{name: "one audio", parts: []part{file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording))}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus, "the transcription job is queued")
		}},
		{name: "two audio files", parts: []part{file("audio", "a.m4a", "audio/x-m4a", []byte(m4aRecording)), file("audio", "b.m4a", "audio/x-m4a", []byte(m4aRecording))},
			wantField: "audio", wantMsg: "Send at most one recording."},
		{name: "audio over NOTE_AUDIO_MAX_BYTES", parts: []part{file("audio", "a.m4a", "audio/x-m4a", []byte(m4aRecording+strings.Repeat("x", 1024)))},
			wantField: "file", wantMsg: "The recording is larger than 0 MiB."},
		{name: "empty request", wantField: "text", wantMsg: "Send some text, a photo or a recording."},
		{name: "focus applied", focus: "trip", parts: []part{field("text", "x"), field("tags", "walk")}, want: func(t *testing.T, n web.ApiNote) {
			require.ElementsMatch(t, []string{"walk", "trip"}, n.Tags)
		}},
		{name: "everything at once", parts: []part{
			field("text", "a passage"), field("kind", "quote"), field("source", "Dune"), field("tags", "sand"), field("captured_at", at),
			file("image", "page.jpg", "image/jpeg", jpg), file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording)),
		}, want: func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "> a passage", n.Body)
			require.Equal(t, "Dune", n.Source)
			require.Equal(t, []string{"sand"}, n.Tags)
			require.Len(t, n.Images, 1)
			require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			user, client := newLoggedIn(t, app)
			key := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey

			if c.focus != "" {
				client.PostJSON("/controls/action/note_focus", map[string]string{"tag": c.focus}).RequireStatus(http.StatusOK)
			}

			for _, b := range c.before {
				postNote(t, app, key, b...).RequireStatus(http.StatusOK)
			}

			resp := postNote(t, app, key, c.parts...)

			if c.wantMsg != "" {
				resp.RequireStatus(http.StatusBadRequest)

				var e apiError
				require.NoError(t, json.Unmarshal([]byte(resp.Body), &e))
				require.Equal(t, apiError{Errors: []string{c.wantMsg}, Field: c.wantField}, e)
				require.Len(t, getNotes(t, app, key, url.Values{"limit": {"100"}}).Notes, len(c.before), "nothing is saved")

				return
			}

			n := decodeData[web.ApiNote](t, resp.RequireStatus(http.StatusOK).Body)
			c.want(t, n)

			saved := getNotes(t, app, key, url.Values{"limit": {"100"}}).Notes
			require.Len(t, saved, len(c.before)+1)
		})
	}

	t.Run("body over NOTE_CAPTURE_MAX_BYTES", func(t *testing.T) {
		t.Parallel()

		user := newUser(t, app)
		key := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey

		postNote(t, app, key, file("image", "big.png", "image/png", make([]byte, 2*65536+1))).RequireStatus(http.StatusRequestEntityTooLarge)
		require.Empty(t, getNotes(t, app, key, url.Values{"limit": {"100"}}).Notes)
	})
}

// Without a speech-to-text backend a recording is kept and nothing waits for
// its transcription.
func TestAPINotes_AudioWithoutSTT(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user := newUser(t, app)
	key := testutil.Must(factory.APIKey(context.Background(), app.DB, user.ID, notesScope))(t).APIKey

	n := decodeData[web.ApiNote](t, postNote(t, app, key, file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording))).RequireStatus(http.StatusOK).Body)
	require.Equal(t, model.NoteTranscriptionNone, n.TranscriptionStatus)
}

// getNotes is GET /api/v1/notes with the query q.
func getNotes(t *testing.T, app *e2e.App, key string, q url.Values) web.ApiGetNotesResponse {
	t.Helper()

	return decodeData[web.ApiGetNotesResponse](t, apiCall(t, app, http.MethodGet, "/api/v1/notes?"+q.Encode(), "Bearer "+key).RequireStatus(http.StatusOK).Body)
}

func noteIDs(r web.ApiGetNotesResponse) []string {
	ids := []string{}
	for _, n := range r.Notes {
		ids = append(ids, n.ID)
	}

	return ids
}

// TestAPINotes_List: GET /api/v1/notes pages the key owner's notes newest
// capture first, with the limit clamped like GET /api/v1/posts, and filters
// like the notes page.
func TestAPINotes_List(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user := newUser(t, app)
	key := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey
	other := newUser(t, app)
	otherKey := testutil.Must(factory.APIKey(ctx, app.DB, other.ID))(t).APIKey

	require.Empty(t, getNotes(t, app, key, url.Values{"limit": {"10"}}).Notes, "empty")

	base := time.Now().Add(-time.Hour)
	note := func(at int, opts ...factory.NoteOpt) string {
		opts = append(opts, factory.CapturedAt(base.Add(time.Duration(at)*time.Minute)))
		return testutil.Must(factory.Note(ctx, app.DB, user.ID, opts...))(t).ID
	}

	// inserted oldest last, so only the ordering puts them newest first
	plain := note(5, factory.WithNoteBody("the river"))
	used := note(4, factory.WithNoteBody("used in a post"), factory.WithNoteTags("walk"))
	walk := note(3, factory.WithNoteTags("walk", "idea"))
	idea := note(2, factory.WithNoteTags("idea"))
	inbox := note(1, factory.WithNoteBody("the sea"))
	all := []string{plain, used, walk, idea, inbox}

	post := testutil.Must(factory.Post(ctx, app.DB, user.ID))(t)
	testutil.Must(factory.NotePostLink(ctx, app.DB, used, post.ID))(t)
	testutil.Must(factory.Note(ctx, app.DB, other.ID, factory.CapturedAt(base)))(t)

	var paged []string

	q := url.Values{"limit": {"2"}}

	for {
		page := getNotes(t, app, key, q)
		require.LessOrEqual(t, len(page.Notes), 2)
		paged = append(paged, noteIDs(page)...)

		if page.Cursor == "" {
			break
		}

		q.Set("cursor", page.Cursor)
	}

	require.Equal(t, all, paged, "every note once, newest first, only the owner's")

	for name, c := range map[string]struct {
		q    url.Values
		want []string
	}{
		"no limit is one":        {url.Values{}, all[:1]},
		"limit below one is one": {url.Values{"limit": {"-5"}}, all[:1]},
		"limit over the max":     {url.Values{"limit": {"1000"}}, all},
		"tag":                    {url.Values{"limit": {"10"}, "tag": {"walk"}}, []string{used, walk}},
		"tags":                   {url.Values{"limit": {"10"}, "tag": {"walk", "idea"}}, []string{walk}},
		"not":                    {url.Values{"limit": {"10"}, "not": {"idea"}}, []string{plain, used, inbox}},
		"unused":                 {url.Values{"limit": {"10"}, "unused": {"true"}}, []string{plain, walk, idea, inbox}},
		"inbox":                  {url.Values{"limit": {"10"}, "inbox": {"true"}}, []string{plain, inbox}},
		"q":                      {url.Values{"limit": {"10"}, "q": {"THE"}}, []string{plain, inbox}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.want, noteIDs(getNotes(t, app, key, c.q)))
		})
	}

	require.Len(t, getNotes(t, app, otherKey, url.Values{"limit": {"10"}}).Notes, 1, "a full key reads the notes too, its owner's only")

	resp := apiCall(t, app, http.MethodGet, "/api/v1/notes?cursor=garbage", "Bearer "+key)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestAPINotes_ShortcutRequests replays the requests the "pcom note"
// Shortcut makes (docs/guide/notes-on-a-phone.md), as Shortcuts' "Get
// Contents of URL" sends a Form body: text fields, and files under the name
// of their field with the file name and type the action that made them gives.
func TestAPINotes_ShortcutRequests(t *testing.T) {
	t.Parallel()

	app := notesApp(t)
	jpg := smallJPEG(t)
	now := "2026-11-07T08:15:42Z" // Shortcuts' "Current Date" as ISO 8601

	cases := []struct {
		name  string
		parts []part
		want  func(t *testing.T, n web.ApiNote)
	}{
		{"Action button: Record Audio", []part{
			field("kind", "note"), field("captured_at", now),
			file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording)),
		}, func(t *testing.T, n web.ApiNote) {
			require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
			require.Equal(t, now, n.CapturedAt.Format(time.RFC3339))
		}},
		{"Share Sheet: text", []part{
			field("kind", "share"), field("captured_at", now), field("text", "Selected passage\nfrom Kindle"),
		}, func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "> Selected passage\n> from Kindle", n.Body)
		}},
		{"Share Sheet: URL", []part{
			field("kind", "share"), field("captured_at", now), field("text", "https://example.test/article"),
		}, func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "https://example.test/article", n.Body)
		}},
		{"Share Sheet: images", []part{
			field("kind", "share"), field("captured_at", now),
			file("image", "IMG_0001.jpeg", "image/jpeg", jpg), file("image", "IMG_0002.jpeg", "image/jpeg", jpg),
		}, func(t *testing.T, n web.ApiNote) {
			require.Len(t, n.Images, 2)
			require.Empty(t, n.Body)
		}},
		{"Quote from photo: Take Photo + Extract Text from Image", []part{
			field("kind", "quote"), field("captured_at", now), field("text", "Text the phone read\noff the page"),
			file("image", "Photo.jpeg", "image/jpeg", jpg),
		}, func(t *testing.T, n web.ApiNote) {
			require.Equal(t, "> Text the phone read\n> off the page", n.Body)
			require.Len(t, n.Images, 1)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			user := newUser(t, app)
			key := testutil.Must(factory.APIKey(context.Background(), app.DB, user.ID, notesScope))(t).APIKey

			c.want(t, decodeData[web.ApiNote](t, postNote(t, app, key, c.parts...).RequireStatus(http.StatusOK).Body))
		})
	}
}
