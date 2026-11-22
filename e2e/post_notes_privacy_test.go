package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/notes"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// TestPostNotes_NoReaderReachesThem: the notes attached to a post, one by
// one or by tag, are their author's alone. None of the post's readers gets
// their text: not a reader of the post, not a share link, not an anonymous
// visitor, RSS, the zip export, the API or the feed.
func TestPostNotes_NoReaderReachesThem(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()

	author := newUser(t, app, factory.WithVisibility(model.ProfileVisibilityPublic))
	reader := newUser(t, app)
	connectUsers(t, app, author, reader)

	const body = "zqxthepostbodyitself"
	post := newPost(t, app, author.ID, factory.WithBody(body), factory.Published(), factory.Visibility(model.PostVisibilityPublic))
	share := testutil.Must(factory.PostShare(ctx, app.DB, post.ID))(t)
	key := testutil.Must(factory.APIKey(ctx, app.DB, author.ID))(t)

	secrets := []string{"zqxattachedonebyone", "zqxbroughtbytag"}
	oneByOne := testutil.Must(factory.Note(ctx, app.DB, author.ID, factory.WithNoteBody(secrets[0])))(t)
	testutil.Must(factory.Note(ctx, app.DB, author.ID, factory.WithNoteBody(secrets[1]), factory.WithNoteTags("walden")))(t)

	svc := notes.New(repo.New(app.DB))
	require.NoError(t, svc.Attach(ctx, author, oneByOne.ID, post.ID))
	testutil.Must(svc.AttachTag(ctx, author, post.ID, "walden"))(t)
	require.Len(t, testutil.Must(svc.ForPost(ctx, author, post.ID))(t).Notes, 2)

	anon := app.Client(t)
	asReader := loginAs(t, app, reader)

	cases := []struct {
		name string
		get  func() *e2e.Response
		// text reads what the reader got
		text func(t *testing.T, resp *e2e.Response) string
	}{
		{"reader of the post", func() *e2e.Response { return asReader.Get("/posts/" + post.ID) }, nil},
		{"share link", func() *e2e.Response { return anon.Get("/shared/" + share.ID) }, nil},
		{"anonymous on a public post", func() *e2e.Response { return anon.Get("/posts/" + post.ID) }, nil},
		{"markdown", func() *e2e.Response { return anon.Get("/posts/" + post.ID + "/md") }, nil},
		{"rss of the author", func() *e2e.Response { return anon.Get("/rss/public/" + author.Username) }, nil},
		{"rss of the site", func() *e2e.Response { return anon.Get("/rss/public") }, nil},
		{"feed", func() *e2e.Response { return asReader.Get("/feed") }, nil},
		{"api", func() *e2e.Response {
			return getWith(t, app, anon, "/api/v1/posts", map[string]string{"Authorization": "Bearer " + key.APIKey})
		}, nil},
		{"zip export", func() *e2e.Response { return anon.Get("/posts/" + post.ID + "/zip") }, func(t *testing.T, resp *e2e.Response) string {
			return zipFileText(t, resp.Body, post.ID+".md")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := requireStatus(t, tc.get(), http.StatusOK)

			text := resp.Body
			if tc.text != nil {
				text = tc.text(t, resp)
			}

			require.Contains(t, text, body, "the response shows the post")

			for _, s := range secrets {
				require.NotContains(t, text, s)
			}
		})
	}

	// the control: the notes are there for their author, and for nobody else
	stranger := newUser(t, app)

	t.Run("the author's notes section", func(t *testing.T) {
		text := requireStatus(t, loginAs(t, app, author).Get("/posts/"+post.ID+"/notes"), http.StatusOK).Body

		for _, s := range secrets {
			require.Contains(t, text, s)
		}
	})

	for name, c := range map[string]*e2e.Client{"reader": asReader, "stranger": loginAs(t, app, stranger)} {
		t.Run("notes section: "+name, func(t *testing.T) {
			resp := requireStatus(t, c.Get("/posts/"+post.ID+"/notes"), http.StatusNotFound)

			for _, s := range secrets {
				require.NotContains(t, resp.Body, s)
			}
		})
	}

	t.Run("notes section: anonymous", func(t *testing.T) {
		resp := requireStatus(t, anon.Get("/posts/"+post.ID+"/notes"), http.StatusFound)
		require.NotContains(t, resp.Body, secrets[0])
	})
}
