package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web"
	"github.com/stretchr/testify/require"
)

func TestShareText(t *testing.T) {
	t.Parallel()

	for _, r := range []struct {
		name, title, text, url, want string
	}{
		{"nothing", "", " ", "", ""},
		{"link alone", "", "", " https://example.com/a ", "https://example.com/a"},
		{"text alone", "", "A passage", "", "A passage"},
		{"all three", "Page", "A passage", "https://example.com/a", "Page\nA passage\nhttps://example.com/a"},
		{"link repeated in the text", "", "A passage https://example.com/a", "https://example.com/a", "A passage https://example.com/a"},
		{"title repeated in the text", "Book", "A passage from Book", "", "A passage from Book"},
		{"the same part twice", "Same", "Same", "", "Same"},
		{"CRLF line breaks", "", "One\r\nTwo\r\n", "", "One\nTwo"},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, r.want, shareText(r.title, r.text, r.url))
		})
	}
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
