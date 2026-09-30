package e2e_test

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// TestPublicFeed: a feed reader subscribing to /rss/public gets the published
// public posts of public profiles, dated by publication, and nothing else.
func TestPublicFeed(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	newPost(t, app, author.ID, factory.Visibility(core.PostVisibilityPublic))
	newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityDirectOnly))

	for _, v := range []core.ProfileVisibility{core.ProfileVisibilityRegisteredUsers, core.ProfileVisibilityConnections} {
		u := newUser(t, app, factory.WithVisibility(v))
		newPost(t, app, u.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	}

	resp := requireStatus(t, app.Client(t).Get("/rss/public"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml"), resp.Header.Get("Content-Type"))

	var doc struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				PubDate string `xml:"pubDate"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	require.NoError(t, xml.Unmarshal([]byte(resp.Body), &doc))
	require.Equal(t, "Public posts on pcom", doc.Channel.Title)

	// a fresh database: the draft, the direct-only post and the public posts of
	// the other profiles are left out
	require.Len(t, doc.Channel.Items, 1, "only the published public post of the public profile")
	it := doc.Channel.Items[0]
	require.Equal(t, public.Subject.String, it.Title)
	require.True(t, strings.HasSuffix(it.Link, "/posts/"+public.ID), it.Link)

	pub, err := time.Parse(time.RFC1123Z, it.PubDate)
	require.NoError(t, err)
	// the factory stamps local time into a timestamp without zone, so compare wall clocks
	const wall = "2006-01-02 15:04:05"
	require.Equal(t, public.PublishedAt.Time.Format(wall), pub.Format(wall))
}
