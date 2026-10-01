package e2e_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// rssTitles fetches a feed and returns its item titles in feed order.
func rssTitles(t *testing.T, app *e2e.App, path string) []string {
	t.Helper()

	resp := app.Client(t).Get(path).RequireStatus(http.StatusOK)

	var doc struct {
		Items []struct {
			Title string `xml:"title"`
		} `xml:"channel>item"`
	}
	require.NoError(t, xml.Unmarshal([]byte(resp.Body), &doc))

	titles := make([]string, 0, len(doc.Items))
	for _, it := range doc.Items {
		titles = append(titles, it.Title)
	}

	return titles
}

// publishRSSPosts publishes more posts than the cap, oldest first, titled
// "Rss 01", "Rss 02", ...; it returns the subject of the newest post and of
// the oldest one the cap still admits.
func publishRSSPosts(t *testing.T, app *e2e.App, authorID string, vis core.PostVisibility) (newest, oldestKept string) {
	t.Helper()

	n := reading.RSSLimit + 5
	for i := 1; i <= n; i++ {
		_, err := factory.Post(context.Background(), app.DB, authorID, factory.Published(),
			factory.Visibility(vis), factory.WithSubject(fmt.Sprintf("Rss %02d", i)))
		require.NoError(t, err)
	}

	return fmt.Sprintf("Rss %02d", n), fmt.Sprintf("Rss %02d", n-reading.RSSLimit+1)
}

// The private feed lists at most RSSLimit items, newest first.
func TestRSSPrivate_Limit(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user := newUser(t, app)
	author := newUser(t, app)
	_, _, err := factory.Connect(ctx, app.DB, user.ID, author.ID)
	require.NoError(t, err)
	token, err := repo.RegenerateFeedToken(ctx, app.DB, user.ID)
	require.NoError(t, err)
	newest, oldest := publishRSSPosts(t, app, author.ID, core.PostVisibilityDirectOnly)

	titles := rssTitles(t, app, "/rss/private/"+token.Token)
	require.Len(t, titles, reading.RSSLimit)
	require.Equal(t, newest, titles[0])
	require.Equal(t, oldest, titles[len(titles)-1])
}

// A public journal's feed lists at most RSSLimit items, newest first.
func TestRSSPublicJournal_Limit(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	newest, oldest := publishRSSPosts(t, app, author.ID, core.PostVisibilityPublic)

	titles := rssTitles(t, app, "/rss/public/"+author.Username)
	require.Len(t, titles, reading.RSSLimit)
	require.Equal(t, newest, titles[0])
	require.Equal(t, oldest, titles[len(titles)-1])
}
