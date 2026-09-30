package e2e_test

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// TestPublicFeed: a feed reader subscribing to /rss/public gets the published
// public posts of public profiles (the Q15 matrix is owned by the reading service).
func TestPublicFeed(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityPublic))
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))
	other := newUser(t, app, factory.WithVisibility(core.ProfileVisibilityRegisteredUsers))
	newPost(t, app, other.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic))

	resp := requireStatus(t, app.Client(t).Get("/rss/public"), http.StatusOK)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml"), resp.Header.Get("Content-Type"))

	var doc struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
				Link  string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	require.NoError(t, xml.Unmarshal([]byte(resp.Body), &doc))
	require.Equal(t, "Public posts on pcom", doc.Channel.Title)

	require.Len(t, doc.Channel.Items, 1, "the registered-users profile's post is left out")
	it := doc.Channel.Items[0]
	require.Equal(t, public.Subject.String, it.Title)
	require.True(t, strings.HasSuffix(it.Link, "/posts/"+public.ID), it.Link)
}
