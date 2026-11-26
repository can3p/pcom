package app_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

// escaped is s as html/template writes it into a page.
func escaped(t *testing.T, s string) string {
	t.Helper()

	var b strings.Builder
	require.NoError(t, template.Must(template.New("").Parse("{{.}}")).Execute(&b, s))

	return b.String()
}

func newDevApp(t *testing.T, dev bool) http.Handler {
	t.Helper()

	return newDevAppWith(t, dev, app.DevInfo{})
}

func newDevAppWith(t *testing.T, dev bool, info app.DevInfo) http.Handler {
	t.Helper()

	return app.New(&app.Deps{
		DB:           testdb.New(t).DB,
		MediaStorage: fakestorage.New(),
		Config: app.Config{
			HTMLDir:     "../../../cmd/web/client/html",
			SessionSalt: "test-salt",
			StaticAsset: func(n string) string { return "/static/" + n },
			DevRoutes:   dev,
			Dev:         info,
		},
	})
}

func getPage(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	return rec
}

func TestDevRoutes_MailPreview(t *testing.T) {
	t.Parallel()

	h := newDevApp(t, true)

	index := getPage(h, "/dev/mail")
	require.Equal(t, http.StatusOK, index.Code)

	for _, m := range mail.All() {
		require.Contains(t, index.Body.String(), m.Name)

		for _, s := range m.Samples {
			require.Contains(t, index.Body.String(), `href="/dev/mail/`+s.Name+`"`)

			rec := getPage(h, "/dev/mail/"+s.Name)
			require.Equal(t, http.StatusOK, rec.Code, s.Name)
			require.Contains(t, rec.Body.String(), "<iframe sandbox srcdoc=", s.Name)
			require.Contains(t, rec.Body.String(), escaped(t, s.Envelope.Mail.Subject), s.Name)
		}
	}

	require.Equal(t, http.StatusNotFound, getPage(h, "/dev/mail/no-such-sample").Code)
}

func TestDevRoutes_Index(t *testing.T) {
	t.Parallel()

	h := newDevAppWith(t, true, app.DevInfo{
		TommyURL:    "http://localhost:8811/ui/",
		PostgresURL: "postgres://pcom:pcom@localhost:5442/pcom",
		Accounts:    []app.DevAccount{{Name: "alice", Email: "alice@example.test", Role: "owns the API key"}},
		APIKey:      "the-key",
	})

	rec := getPage(h, "/dev")
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	for _, want := range []string{
		`href="/dev/mail"`,
		`href="http://localhost:8811/ui/"`,
		"postgres://pcom:pcom@localhost:5442/pcom",
		"alice@example.test",
		"owns the API key",
		"the-key",
	} {
		require.Contains(t, body, want)
	}

	require.NotContains(t, body, "S3:", "an unset address leaves its row out")
}

func TestDevRoutes_FooterLink(t *testing.T) {
	t.Parallel()

	require.Contains(t, getPage(newDevApp(t, true), "/login").Body.String(), `href="/dev"`)
	require.NotContains(t, getPage(newDevApp(t, false), "/login").Body.String(), `href="/dev"`)
}

func TestDevRoutes_Off(t *testing.T) {
	t.Parallel()

	h := newDevApp(t, false)

	require.Equal(t, http.StatusNotFound, getPage(h, "/dev").Code)

	require.Equal(t, http.StatusNotFound, getPage(h, "/dev/mail").Code)
	require.Equal(t, http.StatusNotFound, getPage(h, "/dev/mail/anything").Code)
}
