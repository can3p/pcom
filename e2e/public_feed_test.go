package e2e_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// TestPublicFeed: a feed reader subscribing to /rss/public gets the published
// public posts of public profiles (the Q15 matrix is owned by the reading service).
func TestPublicFeed(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	public := newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	other := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityRegisteredUsers))
	newPost(t, app, other.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic))

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

// The index and a journal answer a "Load more" request from htmx with the
// posts alone and a plain request with the whole page from that cursor; a bad
// cursor is a 400.
func TestPublicLists_CursorPages(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	for i := 1; i <= 35; i++ { // more than one page (reading.DefaultPageSize is 30)
		newPost(t, app, author.ID, factory.Published(), factory.Visibility(model.PostVisibilityPublic),
			factory.WithSubject(fmt.Sprintf("Paged %02d", i)))
	}

	for _, path := range []string{"/", "/users/" + author.Username} {
		t.Run(path, func(t *testing.T) {
			c := app.Client(t)

			href, ok := c.Get(path).RequireStatus(http.StatusOK).Doc().Find("a.btn:contains('Load more')").First().Attr("href")
			require.True(t, ok, "first page has no Load more link")

			req, err := http.NewRequest(http.MethodGet, app.URL+href, nil)
			require.NoError(t, err)
			req.Header.Set("HX-Request", "true")
			frag := c.Do(req).RequireStatus(http.StatusOK)
			require.NotContains(t, frag.Body, "<html")
			require.NotContains(t, frag.Body, `<nav class="nav" aria-label="Main"`)
			require.Contains(t, frag.Body, "Paged 01")
			require.NotContains(t, frag.Body, "Load more", "the last page has no button")

			full := c.Get(href).RequireStatus(http.StatusOK)
			require.Contains(t, full.Body, "<html")
			require.Contains(t, full.Body, `<nav class="nav" aria-label="Main"`)
			require.Contains(t, full.Body, "Paged 01")
			require.NotContains(t, full.Body, "Paged 35", "the page starts at the cursor")

			c.Get(path + "?cursor=garbage").RequireStatus(http.StatusBadRequest)
		})
	}
}
