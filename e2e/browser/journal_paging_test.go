//go:build browser

// A journal pages its posts: "Load more" appends the older ones in place.
package browser_test

import (
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
)

func TestJournal_LoadMore(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t, e2e.WithRealAssets())
	author := browser.NewUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	publishPosts(t, app, author.ID, "Journalpost", model.PostVisibilityPublic, pagedPosts)

	expectLoadsMore(t, browser.Page(t, app), "/users/"+author.Username, "Journalpost")
}
