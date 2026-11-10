package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/web"
	webapp "github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

// apiRouteWant is what a route of the API answers a call whose body is "{}"
// from its owner: path makes the concrete path (a fresh post of the user for
// a :id), full is the status with a full key, and notes says whether a
// notes key gets the same answer instead of 403.
type apiRouteWant struct {
	path  func(t *testing.T, app *e2e.App, userID string) string
	full  int
	notes bool
}

func staticAPIPath(p string) func(*testing.T, *e2e.App, string) string {
	return func(*testing.T, *e2e.App, string) string { return p }
}

func postPath(t *testing.T, app *e2e.App, userID string) string {
	t.Helper()

	return "/api/v1/posts/" + testutil.Must(factory.Post(context.Background(), app.DB, userID))(t).ID
}

// apiRouteWants is every route of /api/v1. TestAPIKeys_EveryRouteByKeyState
// checks it against the router, so a new route fails until it has a row.
var apiRouteWants = map[string]apiRouteWant{
	"GET /api/v1/posts":        {staticAPIPath("/api/v1/posts"), http.StatusOK, false},
	"POST /api/v1/posts":       {staticAPIPath("/api/v1/posts"), http.StatusBadRequest, false},
	"POST /api/v1/posts/:id":   {postPath, http.StatusBadRequest, false},
	"DELETE /api/v1/posts/:id": {postPath, http.StatusOK, false},
	"PUT /api/v1/image":        {staticAPIPath("/api/v1/image"), http.StatusBadRequest, false},
	"POST /api/v1/notes":       {staticAPIPath("/api/v1/notes"), http.StatusBadRequest, true},
	"GET /api/v1/notes":        {staticAPIPath("/api/v1/notes"), http.StatusOK, true},
}

// routerAPIRoutes are the /api/v1 routes of the app's router, built as the
// binary builds it.
func routerAPIRoutes(t *testing.T, app *e2e.App) []string {
	t.Helper()

	router := webapp.New(&webapp.Deps{
		DB:           app.DB,
		MediaStorage: fakestorage.New(),
		Config: webapp.Config{
			HTMLDir:     "../cmd/web/client/html",
			SessionSalt: "test-salt",
			StaticAsset: func(n string) string { return "/static/" + n },
		},
	})

	var routes []string

	for _, r := range router.Routes() {
		if strings.HasPrefix(r.Path, "/api/") {
			routes = append(routes, r.Method+" "+r.Path)
		}
	}

	slices.Sort(routes)

	return routes
}

func notesScope(k *model.UserAPIKey) { k.Scope = model.APIKeyScopeNotes }

// apiCall sends an API request with the Authorization header auth ("" for
// none) and the body "{}".
func apiCall(t *testing.T, app *e2e.App, method, path, auth string) *e2e.Response {
	t.Helper()

	req, err := http.NewRequest(method, app.URL+path, strings.NewReader("{}"))
	require.NoError(t, err)

	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	return app.Client(t).Do(req)
}

// TestAPIKeys_EveryRouteByKeyState: every API route, taken from the router,
// answers 401 without a valid key, whatever the shape of the header; a
// notes key gets 403 outside the notes API and the route's own answer in
// it; a full key gets the route's own answer everywhere.
func TestAPIKeys_EveryRouteByKeyState(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user, owner := newLoggedIn(t, app)

	want := make([]string, 0, len(apiRouteWants))
	for k := range apiRouteWants {
		want = append(want, k)
	}

	slices.Sort(want)
	require.Equal(t, want, routerAPIRoutes(t, app), "every API route has a row in apiRouteWants")

	full := testutil.Must(factory.APIKey(ctx, app.DB, user.ID))(t).APIKey
	notesKey := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey
	revoked := testutil.Must(factory.APIKey(ctx, app.DB, user.ID))(t)
	owner.PostJSON("/controls/action/revoke_api_key", map[string]string{"id": revoked.ID}).RequireStatus(http.StatusOK)
	// the private feed token is a secret of the same user, but no API key
	feedToken := testutil.Must(repo.RegenerateFeedToken(ctx, app.DB, user.ID))(t).Token

	const own = 0 // the route's own answer

	states := []struct {
		name string
		auth string
		want int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"Bearer alone", "Bearer", http.StatusUnauthorized},
		{"Bearer and a space", "Bearer ", http.StatusUnauthorized},
		{"lowercase scheme", "bearer " + full, http.StatusUnauthorized},
		{"other scheme", "Basic " + full, http.StatusUnauthorized},
		{"two spaces", "Bearer  " + full, http.StatusUnauthorized},
		{"extra token", "Bearer " + full + " x", http.StatusUnauthorized},
		{"key alone", full, http.StatusUnauthorized},
		{"unknown key", "Bearer not-a-key", http.StatusUnauthorized},
		{"feed token", "Bearer " + feedToken, http.StatusUnauthorized},
		{"revoked key", "Bearer " + revoked.APIKey, http.StatusUnauthorized},
		{"notes key", "Bearer " + notesKey, http.StatusForbidden},
		{"full key", "Bearer " + full, own},
	}

	for route, r := range apiRouteWants {
		method := strings.Fields(route)[0]

		for _, s := range states {
			t.Run(route+"/"+s.name, func(t *testing.T) {
				t.Parallel()

				status := s.want

				switch {
				case status == own, status == http.StatusForbidden && r.notes:
					status = r.full
				}

				resp := apiCall(t, app, method, r.path(t, app, user.ID), s.auth)
				resp.RequireStatus(status)

				if status == http.StatusUnauthorized {
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

	refused := stranger.PostJSON("/controls/action/revoke_api_key", map[string]string{"id": keys[0].ID}).RequireStatus(http.StatusBadRequest)
	require.JSONEq(t, `{"explanation":"Not found"}`, refused.Body)
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
	Field  string   `json:"field,omitempty"`
}

func requireAPIError(t *testing.T, resp *e2e.Response, status int, want apiError) {
	t.Helper()

	resp.RequireStatus(status)

	var got apiError
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &got))
	require.Equal(t, want, got)
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

// A recording as phones make it.
const (
	m4aRecording = "\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00isomM4A mp42 and the rest of a memo"
)

// notesApp starts the app the notes API tests share: transcription on,
// against a backend nobody answers at, so a recording's note stays pending.
func notesApp(t *testing.T, opts ...e2e.Option) *e2e.App {
	t.Helper()

	return e2e.Start(t, append([]e2e.Option{
		e2e.WithEnv("STT_PROVIDER", "openai-compat"),
		e2e.WithEnv("STT_OPENAI_COMPAT_ENDPOINT", "http://127.0.0.1:9/stt"),
		e2e.WithEnv("STT_OPENAI_COMPAT_KEY", "unused"),
	}, opts...)...)
}

// stored is what a user has in the database after captures: notes, photos,
// image uploads, recordings and transcription jobs.
type stored struct {
	notes      []*model.Note
	images     []*model.NoteImage
	uploads    []*model.MediaUpload
	recordings []*model.NoteRecording
	jobs       int
}

func storedOf(t *testing.T, app *e2e.App, userID string) stored {
	t.Helper()

	ctx := context.Background()

	return stored{
		notes:      testutil.Must(factory.ListNotes(ctx, app.DB, userID))(t),
		images:     testutil.Must(factory.ListNoteImages(ctx, app.DB, userID))(t),
		uploads:    testutil.Must(factory.ListMediaUploads(ctx, app.DB, userID))(t),
		recordings: testutil.Must(factory.ListNoteRecordings(ctx, app.DB, userID))(t),
		jobs:       testutil.Must(factory.CountNoteJobs(ctx, app.DB, notes.TranscribeArgs{}.Kind(), userID))(t),
	}
}

// requireStoredAs checks that n, the API's answer, is the note stored last
// for userID: its id, and its photos as their URLs in order.
func requireStoredAs(t *testing.T, app *e2e.App, userID string, n web.ApiNote) stored {
	t.Helper()

	s := storedOf(t, app, userID)
	ids := make([]string, 0, len(s.notes))

	for _, sn := range s.notes {
		ids = append(ids, sn.ID)
	}

	require.Contains(t, ids, n.ID)

	fnames := map[string]string{}
	for _, u := range s.uploads {
		fnames[u.ID] = u.UploadedFname
	}

	urls := make([]string, len(n.Images))

	for _, img := range s.images {
		if img.NoteID == n.ID {
			urls[img.Position] = app.URL + "/user-media/" + fnames[img.UploadID]
		}
	}

	require.Equal(t, urls, n.Images)

	return s
}

// TestAPINotes_Create: every field of POST /api/v1/notes, alone and
// together, and the refusals, each with the field to fix and nothing stored.
func TestAPINotes_Create(t *testing.T) {
	t.Parallel()

	app := notesApp(t)
	ctx := context.Background()
	png1, jpg := smallPNG(t), smallJPEG(t)
	at := "2026-11-07T09:30:00+01:00"
	long := strings.Repeat("x", notes.DefaultSourceTitleMaxLength+1)
	tooLong := []byte(m4aRecording + strings.Repeat("x", notes.DefaultAudioMaxBytes))

	cases := []struct {
		name string
		// before are requests sent first with the same key.
		before    [][]part
		parts     []part
		wantField string
		wantMsg   string
		want      func(t *testing.T, n web.ApiNote, s stored)
	}{
		{name: "text only", parts: []part{field("text", " hello ")}, want: func(t *testing.T, n web.ApiNote, s stored) {
			require.Equal(t, "hello", n.Body, "typed text is kept as typed")
			require.Empty(t, n.Tags)
			require.Empty(t, n.Source)
			require.Empty(t, n.Images)
			require.Equal(t, model.NoteTranscriptionNone, n.TranscriptionStatus)
			require.WithinDuration(t, time.Now(), n.CapturedAt, time.Minute, "no captured_at is now")
			require.Empty(t, s.uploads)
			require.Empty(t, s.recordings)
			require.Zero(t, s.jobs)
		}},
		{name: "kind share", parts: []part{field("text", "line one\nline two"), field("kind", "share")}, want: func(t *testing.T, n web.ApiNote, _ stored) {
			require.Equal(t, "> line one\n> line two", n.Body)
			require.Empty(t, n.Source)
		}},
		{name: "kind quote", parts: []part{field("text", "a passage"), field("kind", "quote"), field("source", "Dune")}, want: func(t *testing.T, n web.ApiNote, _ stored) {
			require.Equal(t, "> a passage", n.Body)
			require.Equal(t, "Dune", n.Source)
		}},
		{name: "quote without source takes the previous quote's", before: [][]part{{field("text", "first"), field("kind", "quote"), field("source", "Dune")}},
			parts: []part{field("text", "second"), field("kind", "quote")}, want: func(t *testing.T, n web.ApiNote, _ stored) {
				require.Equal(t, "Dune", n.Source)
			}},
		{name: "lone URL stays bare", parts: []part{field("text", " https://example.test/page "), field("kind", "share")}, want: func(t *testing.T, n web.ApiNote, _ stored) {
			require.Equal(t, "https://example.test/page", n.Body)
		}},
		{name: "unknown kind", parts: []part{field("text", "x"), field("kind", "poem")}, wantField: "kind", wantMsg: "kind is note, quote or share."},
		{name: "tags repeated and comma-separated, normalized", parts: []part{field("text", "x #Inline"), field("tags", "Walk, #Idea"), field("tags", "book")},
			want: func(t *testing.T, n web.ApiNote, _ stored) {
				require.ElementsMatch(t, []string{"walk", "idea", "book", "inline"}, n.Tags)
			}},
		{name: "too many tags", parts: []part{field("text", "x"), field("tags", "a,b,c,d,e,f,g,h,i,j,k")}, wantField: "tags",
			wantMsg: fmt.Sprintf("A note can have at most %d tags.", notes.DefaultTagsMax)},
		{name: "tag too long", parts: []part{field("text", "x"), field("tags", strings.Repeat("t", notes.DefaultTagMaxLength+1))}, wantField: "tags",
			wantMsg: fmt.Sprintf("A tag can be at most %d characters long.", notes.DefaultTagMaxLength)},
		{name: "captured_at", parts: []part{field("text", "x"), field("captured_at", at)}, want: func(t *testing.T, n web.ApiNote, _ stored) {
			want, err := time.Parse(time.RFC3339, at)
			require.NoError(t, err)
			require.True(t, want.Equal(n.CapturedAt), "%s", n.CapturedAt)
		}},
		{name: "captured_at invalid", parts: []part{field("text", "x"), field("captured_at", "yesterday")}, wantField: "captured_at",
			wantMsg: "captured_at is not an RFC 3339 time, such as 2026-11-07T09:30:00+01:00."},
		{name: "source new", parts: []part{field("text", "x"), field("source", "The Book")}, want: func(t *testing.T, n web.ApiNote, _ stored) {
			require.Equal(t, "The Book", n.Source)
		}},
		{name: "source existing is reused", before: [][]part{{field("text", "x"), field("source", "The Book")}},
			parts: []part{field("text", "y"), field("source", " the  book ")}, want: func(t *testing.T, n web.ApiNote, s stored) {
				require.Equal(t, "The Book", n.Source, "found by title whatever the case and spacing")
				require.Equal(t, *s.notes[0].SourceID, *s.notes[1].SourceID)
			}},
		{name: "source too long", parts: []part{field("text", "x"), field("source", long)}, wantField: "source",
			wantMsg: fmt.Sprintf("A title can be at most %d characters long.", notes.DefaultSourceTitleMaxLength)},
		{name: "several images", parts: []part{file("image", "a.png", "image/png", png1), file("image", "b.jpg", "image/jpeg", jpg)},
			want: func(t *testing.T, n web.ApiNote, s stored) {
				require.Empty(t, n.Body)
				require.Len(t, n.Images, 2)
				require.True(t, strings.HasSuffix(n.Images[0], ".png"), "in the order sent")
				require.True(t, strings.HasSuffix(n.Images[1], ".jpg"))
				require.Len(t, s.uploads, 2)
			}},
		{name: "a non-image image", parts: []part{file("image", "a.png", "image/png", png1), file("image", "notes.txt", "image/png", []byte("plain text"))},
			wantField: "image", wantMsg: "pcom keeps photos in JPEG, PNG or WebP."},
		{name: "one audio", parts: []part{file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording))}, want: func(t *testing.T, n web.ApiNote, s stored) {
			require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
			require.Empty(t, s.uploads, "a recording is not an image upload")
			require.Len(t, s.recordings, 1)
			require.Equal(t, s.recordings[0].ID, *s.notes[0].RecordingID)
			require.Equal(t, 1, s.jobs, "one notes.transcribe job is queued")
		}},
		{name: "two audio files", parts: []part{file("audio", "a.m4a", "audio/x-m4a", []byte(m4aRecording)), file("audio", "b.m4a", "audio/x-m4a", []byte(m4aRecording))},
			wantField: "audio", wantMsg: "Send at most one recording."},
		{name: "audio over NOTE_AUDIO_MAX_BYTES", parts: []part{file("audio", "a.m4a", "audio/x-m4a", tooLong)},
			wantField: "audio", wantMsg: fmt.Sprintf("The recording is larger than %d MiB.", notes.DefaultAudioMaxBytes>>20)},
		{name: "empty request", wantField: "text", wantMsg: "Send some text, a photo or a recording."},
		{name: "everything at once", parts: []part{
			field("text", "a passage"), field("kind", "quote"), field("source", "Dune"), field("tags", "sand"), field("captured_at", at),
			file("image", "page.jpg", "image/jpeg", jpg), file("audio", "Recording.m4a", "audio/x-m4a", []byte(m4aRecording)),
		}, want: func(t *testing.T, n web.ApiNote, s stored) {
			require.Equal(t, "> a passage", n.Body)
			require.Equal(t, "Dune", n.Source)
			require.Equal(t, []string{"sand"}, n.Tags)
			require.Len(t, n.Images, 1)
			require.Equal(t, model.NoteTranscriptionPending, n.TranscriptionStatus)
			require.Len(t, s.uploads, 1, "the photo")
			require.Len(t, s.recordings, 1)
			require.Equal(t, 1, s.jobs)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			user := newUser(t, app)
			key := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey

			for _, b := range c.before {
				postNote(t, app, key, b...).RequireStatus(http.StatusOK)
			}

			before := storedOf(t, app, user.ID)
			resp := postNote(t, app, key, c.parts...)

			if c.wantMsg != "" {
				requireAPIError(t, resp, http.StatusBadRequest, apiError{Errors: []string{c.wantMsg}, Field: c.wantField})
				require.Equal(t, before, storedOf(t, app, user.ID), "nothing is stored")
				require.Zero(t, testutil.Must(factory.CountOrphanNoteJobs(ctx, app.DB))(t), "no job is queued")

				return
			}

			n := decodeData[web.ApiNote](t, resp.RequireStatus(http.StatusOK).Body)
			s := requireStoredAs(t, app, user.ID, n)
			require.Len(t, s.notes, len(c.before)+1)
			c.want(t, n, s)
		})
	}
}

// The notes cap raises the body cap of POST /api/v1/notes alone: a body
// over MAX_BODY_SIZE passes it, one over MAX_BODY_SIZE plus
// NOTE_CAPTURE_MAX_BYTES gets 413 from the notes API, as JSON.
func TestAPINotes_BodyCap(t *testing.T) {
	t.Parallel()

	const maxBody, maxCapture = 2 << 20, 1 << 20

	app := notesApp(t, e2e.WithEnv("MAX_BODY_SIZE", fmt.Sprint(maxBody)), e2e.WithEnv("NOTE_CAPTURE_MAX_BYTES", fmt.Sprint(maxCapture)))
	user := newUser(t, app)
	key := testutil.Must(factory.APIKey(context.Background(), app.DB, user.ID, notesScope))(t).APIKey

	between := postNote(t, app, key, file("image", "big.png", "image/png", make([]byte, maxBody+maxCapture/2)))
	requireAPIError(t, between, http.StatusBadRequest, apiError{Errors: []string{"pcom keeps photos in JPEG, PNG or WebP."}, Field: "image"})

	over := postNote(t, app, key, file("image", "big.png", "image/png", make([]byte, maxBody+maxCapture+1)))
	requireAPIError(t, over, http.StatusRequestEntityTooLarge, apiError{Errors: []string{"The capture is too large."}})

	require.Equal(t, stored{}, storedOf(t, app, user.ID))
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

	s := requireStoredAs(t, app, user.ID, n)
	require.Empty(t, s.uploads)
	require.Len(t, s.recordings, 1)
	require.Zero(t, s.jobs, "no transcription job is queued")
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
	sea := note(1, factory.WithNoteBody("the sea"))
	all := []string{plain, used, walk, idea, sea}

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
		"no limit is one":                 {url.Values{}, all[:1]},
		"limit below one is one":          {url.Values{"limit": {"-5"}}, all[:1]},
		"tag":                             {url.Values{"limit": {"10"}, "tag": {"walk"}}, []string{used, walk}},
		"tags":                            {url.Values{"limit": {"10"}, "tag": {"walk", "idea"}}, []string{walk}},
		"not":                             {url.Values{"limit": {"10"}, "not": {"idea"}}, []string{plain, used, sea}},
		"unused and inbox are no filters": {url.Values{"limit": {"10"}, "unused": {"true"}, "inbox": {"true"}}, all},
		"q":                               {url.Values{"limit": {"10"}, "q": {"THE"}}, []string{plain, sea}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.want, noteIDs(getNotes(t, app, key, c.q)))
		})
	}

	require.Len(t, getNotes(t, app, otherKey, url.Values{"limit": {"10"}}).Notes, 1, "a full key reads the notes too, its owner's only")

	resp := apiCall(t, app, http.MethodGet, "/api/v1/notes?cursor=garbage", "Bearer "+key)
	requireAPIError(t, resp, http.StatusBadRequest, apiError{Errors: []string{"This page link is not valid."}, Field: "cursor"})
}

// A page holds at most notes.ListMax notes, however many are asked for.
func TestAPINotes_ListMax(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user := newUser(t, app)
	key := testutil.Must(factory.APIKey(ctx, app.DB, user.ID, notesScope))(t).APIKey
	base := time.Now().Add(-time.Hour)

	for i := range notes.ListMax + 1 {
		testutil.Must(factory.Note(ctx, app.DB, user.ID, factory.CapturedAt(base.Add(time.Duration(i)*time.Second))))(t)
	}

	for _, limit := range []int{notes.ListMax, notes.ListMax + 1, 1000} {
		page := getNotes(t, app, key, url.Values{"limit": {fmt.Sprint(limit)}})
		require.Len(t, page.Notes, notes.ListMax, "limit %d", limit)
		require.NotEmpty(t, page.Cursor, "limit %d: one more note follows", limit)

		rest := getNotes(t, app, key, url.Values{"limit": {fmt.Sprint(limit)}, "cursor": {page.Cursor}})
		require.Len(t, rest.Notes, 1)
		require.Empty(t, rest.Cursor)
	}
}

// TestAPINotes_ShortcutRequests replays the requests the "pcom note"
// Shortcut makes (docs/guide/notes-on-a-phone.md), as Shortcuts' "Get
// Contents of URL" sends a Form body: text fields, and files under the name
// of their field with the file name and type the action that made them gives.
// They are built here until bodies recorded on the owner's iPhone replace
// them (testdata/shortcut/README.md).
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

			n := decodeData[web.ApiNote](t, postNote(t, app, key, c.parts...).RequireStatus(http.StatusOK).Body)
			requireStoredAs(t, app, user.ID, n)
			c.want(t, n)
		})
	}
}

// A HEIC photo is stored as the JPEG it is converted to.
func TestAPINotes_HEIC(t *testing.T) {
	t.Parallel()

	heic := testutil.Must(os.ReadFile("../pkg/media/testdata/photo.heic"))(t)
	if _, err := media.HEICToJPEG(heic); errors.Is(err, media.ErrHEICUnsupported) {
		if os.Getenv("CI") != "" {
			t.Fatal("libvips can't decode HEIC: CI must decode HEIC (libheif with an HEVC decoder)")
		}

		t.Skip("libvips can't decode HEIC")
	}

	app := notesApp(t)
	user := newUser(t, app)
	key := testutil.Must(factory.APIKey(context.Background(), app.DB, user.ID, notesScope))(t).APIKey

	n := decodeData[web.ApiNote](t, postNote(t, app, key, file("image", "IMG_0001.HEIC", "image/heic", heic)).RequireStatus(http.StatusOK).Body)
	st := requireStoredAs(t, app, user.ID, n)

	require.Len(t, n.Images, 1)
	require.True(t, strings.HasSuffix(n.Images[0], ".jpg"), n.Images[0])
	require.Len(t, st.uploads, 1)
	require.Equal(t, "image/jpeg", st.uploads[0].ContentType)
}
