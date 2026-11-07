package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBodyLimits(t *testing.T) {
	const maxBody, maxUpload, maxImport, maxCapture = 100, 1000, 5000, 2000

	router := gin.New()
	router.Use(bodyLimits(maxBody, maxUpload, maxImport, maxCapture))

	read := func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}

		c.Status(http.StatusOK)
	}

	for _, p := range []string{"/form/login", uploadMediaPath, apiImagePath, importPath, noteAudioPath, noteSharePath, apiNotesPath} {
		router.POST(p, read)
	}

	router.PUT(apiImagePath, read)

	for _, r := range []struct {
		method, path string
		size         int
		want         int
	}{
		{http.MethodPost, "/form/login", maxBody, http.StatusOK},
		{http.MethodPost, "/form/login", maxBody + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPost, uploadMediaPath, maxBody + maxUpload, http.StatusOK},
		{http.MethodPost, uploadMediaPath, maxBody + maxUpload + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPut, apiImagePath, maxBody + maxUpload, http.StatusOK},
		{http.MethodPut, apiImagePath, maxBody + maxUpload + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPost, importPath, maxBody + maxImport, http.StatusOK},
		{http.MethodPost, importPath, maxBody + maxImport + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPost, noteAudioPath, maxBody + maxCapture, http.StatusOK},
		{http.MethodPost, noteAudioPath, maxBody + maxCapture + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPost, noteSharePath, maxBody + maxCapture, http.StatusOK},
		{http.MethodPost, noteSharePath, maxBody + maxCapture + 1, http.StatusRequestEntityTooLarge},
		{http.MethodPost, apiNotesPath, maxBody + maxCapture, http.StatusOK},
		{http.MethodPost, apiNotesPath, maxBody + maxCapture + 1, http.StatusRequestEntityTooLarge},
	} {
		req := httptest.NewRequest(r.method, r.path, bytes.NewReader(make([]byte, r.size)))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		require.Equal(t, r.want, rec.Code, "%s %s with %d bytes", r.method, r.path, r.size)
	}
}

// TestBodyLimits_UploadPaths checks the paths bodyLimits raises the cap on
// are routes of the app, so renaming a route can't drop its upload cap.
func TestBodyLimits_UploadPaths(t *testing.T) {
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

	routes := map[string]bool{}
	for _, r := range router.Routes() {
		routes[r.Method+" "+r.Path] = true
	}

	// The notes capture routes other tasks of the notes wave add; once one
	// exists, its entry here goes.
	notYet := map[string]string{
		"POST " + noteSharePath: "the share target (F15 T8)",
		"POST " + apiNotesPath:  "the notes API (F15 T5)",
	}

	for _, want := range []string{"POST " + uploadMediaPath, "PUT " + apiImagePath, "POST " + importPath, "POST " + noteAudioPath, "POST " + noteSharePath, "POST " + apiNotesPath} {
		if by, ok := notYet[want]; ok {
			require.False(t, routes[want], "%s exists now: drop it from notYet", want)
			t.Logf("%s is not checked: %s doesn't exist yet", want, by)

			continue
		}

		require.True(t, routes[want], "no route %s", want)
	}
}

func TestNewServer_Timeouts(t *testing.T) {
	srv := NewServer(":0", http.NotFoundHandler(), ServerTimeouts{Write: time.Minute})

	require.Equal(t, DefaultReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, DefaultReadTimeout, srv.ReadTimeout)
	require.Equal(t, time.Minute, srv.WriteTimeout)
	require.Equal(t, DefaultIdleTimeout, srv.IdleTimeout)
}
