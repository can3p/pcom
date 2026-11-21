package e2e_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

// GET /write/preview draws the author's own scratch pad: nothing while it is
// empty, the saved text after an autosave, and never somebody else's.
func TestWritePreview_ShowsOnlyTheAuthorsPad(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	_, author := newLoggedIn(t, app)
	_, other := newLoggedIn(t, app)

	author.Get("/write/preview").RequireStatus(http.StatusNotFound)

	author.PostForm("/controls/form/edit_post", url.Values{
		"subject": {"Unsaved thoughts"}, "body": {"Some *words*"}, "scratch_rev": {"0"},
		"visibility": {string(model.PostVisibilityDirectOnly)}, "save_action": {"autosave"},
	}).RequireStatus(http.StatusOK)

	doc := author.Get("/write/preview").RequireStatus(http.StatusOK).Doc()
	require.Equal(t, "Unsaved thoughts", doc.Find(".us-post-header").Text())
	require.Equal(t, "words", doc.Find(".us-post-body em").Text())

	other.Get("/write/preview").RequireStatus(http.StatusNotFound)
}
