package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	imgpng "image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web"
	"github.com/stretchr/testify/require"
)

// shareTextCase is a row of testdata/share_text.json, which the browser test
// of the share target runs through notes_sw.js too, so both join a share
// alike.
type shareTextCase struct {
	Name, Title, Text, URL, Want string
}

func TestShareText(t *testing.T) {
	t.Parallel()

	var cases []shareTextCase
	require.NoError(t, json.Unmarshal(testutil.Must(os.ReadFile("testdata/share_text.json"))(t), &cases))
	require.NotEmpty(t, cases)

	for _, r := range cases {
		t.Run(r.Name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, r.Want, shareText(r.Title, r.Text, r.URL))
		})
	}
}

// shareFile is a file of a share form, in the field of its kind.
type shareFile struct {
	field, name, contentType string
	body                     []byte
}

// postShare posts a multipart share as user to the share route and returns
// the answer.
func postShare(t *testing.T, svc *notes.Service, db *sql.DB, userID string, fields map[string]string, files ...shareFile) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer

	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}

	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.field, f.name))
		h.Set("Content-Type", f.contentType)

		w, err := mw.CreatePart(h)
		require.NoError(t, err)

		_, err = w.Write(f.body)
		require.NoError(t, err)
	}

	require.NoError(t, mw.Close())

	c, rec := ginctx.New(t, http.MethodPost, noteSharePath, &body, ginctx.WithUser(t, db, userID))
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())

	shareNote(svc)(c)
	c.Writer.WriteHeaderNow()

	return rec
}

func TestShareRoute(t *testing.T) {
	t.Parallel()

	const maxPhoto = 4 << 10

	ctx := context.Background()
	db := testdb.New(t).DB
	store := repo.New(db)
	svc := notes.New(store, notes.WithJobs(fakequeue.New()), notes.WithMediaStorage(fakestorage.New()), notes.WithMaxUploadSize(maxPhoto))

	var png bytes.Buffer
	require.NoError(t, imgpng.Encode(&png, image.NewRGBA(image.Rect(0, 0, 4, 4))))

	ogg := append([]byte("OggS"), make([]byte, 64)...)
	heic := append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), make([]byte, 64)...)

	photo := func(name, contentType string, body []byte) shareFile {
		return shareFile{web.CaptureFieldImage, name, contentType, body}
	}
	audio := func(body []byte) shareFile {
		return shareFile{web.CaptureFieldAudio, "memo.ogg", "audio/ogg", body}
	}

	// saved checks the share was saved as the user's note, which the answer
	// sends the phone to, and returns it.
	saved := func(t *testing.T, rec *httptest.ResponseRecorder, userID string) *model.Note {
		t.Helper()

		require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())

		loc, err := url.Parse(rec.Header().Get("Location"))
		require.NoError(t, err)
		require.Equal(t, web.NoteCapturePath, loc.Path)

		n, err := store.NoteWithRelations(ctx, loc.Query().Get(web.CaptureSharedParam))
		require.NoError(t, err)
		require.Equal(t, userID, n.UserID)

		return n
	}

	refused := func(t *testing.T, rec *httptest.ResponseRecorder, userID, msg string) {
		t.Helper()

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), msg)

		rows, err := store.ListNotes(ctx, repo.NoteFilter{UserID: userID, ShowHidden: true, Limit: 10})
		require.NoError(t, err)
		require.Empty(t, rows)
	}

	newUser := func(t *testing.T) string {
		t.Helper()

		return testutil.Must(factory.User(ctx, db))(t).ID
	}

	t.Run("an image", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		n := saved(t, postShare(t, svc, db, user, nil, photo("p.png", "image/png", png.Bytes())), user)
		require.Empty(t, n.Body)
		require.Len(t, n.Images, 1)
		require.Nil(t, n.AudioUploadID)
	})

	t.Run("an audio file", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		n := saved(t, postShare(t, svc, db, user, nil, audio(ogg)), user)
		require.NotNil(t, n.AudioUploadID)
		require.Empty(t, n.Images)
	})

	t.Run("two audio files", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		refused(t, postShare(t, svc, db, user, nil, audio(ogg), audio(ogg)), user, "Send at most one recording.")
	})

	t.Run("a photo over its cap", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		big := append(bytes.Clone(png.Bytes()), make([]byte, maxPhoto)...)
		refused(t, postShare(t, svc, db, user, nil, photo("p.png", "image/png", big)), user, "A photo can be at most")
	})

	t.Run("HEIC that is not a photo", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		refused(t, postShare(t, svc, db, user, nil, photo("p.heic", "image/heic", heic)), user, "keeps photos")
	})

	t.Run("not an image", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		refused(t, postShare(t, svc, db, user, nil, photo("p.png", "image/png", []byte("plain text, not a photo"))), user, "JPEG, PNG or WebP")
	})

	t.Run("tags, source and captured_at are ignored", func(t *testing.T) {
		t.Parallel()

		user := newUser(t)
		before := time.Now()
		n := saved(t, postShare(t, svc, db, user, map[string]string{
			shareFieldText:             "A passage",
			web.CaptureFieldTags:       "forged,tags",
			web.CaptureFieldSource:     "A forged book",
			web.CaptureFieldCapturedAt: "2001-02-03T04:05:06Z",
			web.CaptureFieldKind:       "note",
		}), user)

		require.Equal(t, "> A passage", n.Body)
		require.Empty(t, n.Tags)
		require.Nil(t, n.Source)
		require.WithinDuration(t, before, n.CapturedAt, time.Minute)

		// a malformed captured_at is ignored as well
		saved(t, postShare(t, svc, db, user, map[string]string{shareFieldText: "Another", web.CaptureFieldCapturedAt: "yesterday"}), user)
	})
}

// TestManifest checks the served manifest has what an installed app and its
// share target need, with its icons as asset URLs, and that the pages it
// opens and the share action are routes of the app.
func TestManifest(t *testing.T) {
	t.Parallel()

	router := New(&Deps{
		DB:           testdb.New(t).DB,
		MediaStorage: fakestorage.New(),
		Config: Config{
			HTMLDir:     "../../../cmd/web/client/html",
			SessionSalt: "test-salt",
			StaticAsset: func(n string) string { return "/static/" + n },
		},
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, manifestPath, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/manifest+json", rec.Header().Get("Content-Type"))

	type param struct {
		Name   string   `json:"name"`
		Accept []string `json:"accept"`
	}

	var m struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		StartURL  string `json:"start_url"`
		Scope     string `json:"scope"`
		Display   string `json:"display"`
		Icons     []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
		} `json:"icons"`
		Shortcuts []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"shortcuts"`
		ShareTarget struct {
			Action  string `json:"action"`
			Method  string `json:"method"`
			Enctype string `json:"enctype"`
			Params  struct {
				Title string  `json:"title"`
				Text  string  `json:"text"`
				URL   string  `json:"url"`
				Files []param `json:"files"`
			} `json:"params"`
		} `json:"share_target"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))

	require.Equal(t, "pcom", m.Name)
	require.NotEmpty(t, m.ShortName)
	require.Equal(t, "/controls/notes/new", m.StartURL)
	require.Equal(t, "/", m.Scope)
	require.Equal(t, "standalone", m.Display)

	var sizes []string
	for _, i := range m.Icons {
		asset, ok := strings.CutPrefix(i.Src, "/static/")
		require.True(t, ok, "icon %s is not an asset URL", i.Src)
		require.FileExists(t, "../../../cmd/web/client/"+asset)

		sizes = append(sizes, i.Sizes)
	}

	require.Subset(t, sizes, []string{"192x192", "512x512"})

	starts := map[string]string{}
	for _, s := range m.Shortcuts {
		u, err := url.Parse(s.URL)
		require.NoError(t, err)
		require.Equal(t, m.StartURL, u.Path, s.Name)

		starts[s.Name] = u.Query().Get("start")
	}

	require.Equal(t, map[string]string{"Dictate": web.CaptureStartVoice, "Photo": web.CaptureStartPhoto}, starts)

	st := m.ShareTarget
	require.Equal(t, noteSharePath, st.Action)
	require.Equal(t, http.MethodPost, st.Method)
	require.Equal(t, "multipart/form-data", st.Enctype)
	require.Equal(t, [3]string{shareFieldTitle, shareFieldText, shareFieldURL}, [3]string{st.Params.Title, st.Params.Text, st.Params.URL})
	require.Equal(t, []param{
		{Name: web.CaptureFieldAudio, Accept: []string{"audio/*"}},
		{Name: web.CaptureFieldImage, Accept: []string{"image/*"}},
	}, st.Params.Files)

	routes := map[string]bool{}
	for _, r := range router.Routes() {
		routes[r.Method+" "+r.Path] = true
	}

	require.True(t, routes["GET "+m.StartURL], "no route for start_url")
	require.True(t, routes[st.Method+" "+st.Action], "no route for the share action")
}
