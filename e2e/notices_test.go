package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

func noticeRows(t *testing.T, app *e2e.App, userID string) int {
	t.Helper()

	var n int

	require.NoError(t, app.DB.QueryRowContext(context.Background(), `SELECT count(*) FROM user_notices WHERE user_id = $1`, userID).Scan(&n))

	return n
}

// An unknown notice is refused with its message and stores nothing; the real
// one is stored once, for the account.
func TestDismissNotice(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	user, c := newLoggedIn(t, app)

	refused := c.PostJSON("/controls/action/dismiss_notice", map[string]string{"notice": "nope"}).RequireStatus(http.StatusBadRequest)
	require.JSONEq(t, `{"explanation":"Unknown notice."}`, refused.Body)
	require.Zero(t, noticeRows(t, app, user.ID))

	c.PostJSON("/controls/action/dismiss_notice", map[string]string{"notice": "scratch_pad"}).RequireStatus(http.StatusOK)
	require.Equal(t, 1, noticeRows(t, app, user.ID))
}

// The drafts table names a draft by its subject, else by the start of its
// body, else "No subject".
func TestControls_DraftNames(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)

	titled := testutil.Must(factory.Post(ctx, app.DB, user.ID, factory.WithSubject("My subject"), factory.WithBody("body of the titled one")))(t)
	bare := testutil.Must(factory.Post(ctx, app.DB, user.ID, factory.WithSubject(""), factory.WithBody("Just **some** words")))(t)
	empty := testutil.Must(factory.Post(ctx, app.DB, user.ID, factory.WithSubject(""), factory.WithBody("")))(t)

	doc := c.Get("/controls/").RequireStatus(http.StatusOK).Doc()
	text := func(id string) string {
		return doc.Find(`a[href$="/posts/` + id + `/edit"]`).Text()
	}

	require.Equal(t, "My subject", text(titled.ID))
	require.Equal(t, "Just some words", text(bare.ID))
	require.Equal(t, "No subject", text(empty.ID))
}

// insertPad stores a scratch pad of the user, for the prompt when promptID
// isn't empty.
func insertPad(t *testing.T, app *e2e.App, userID, promptID, subject, body string) {
	t.Helper()

	_, err := app.DB.ExecContext(context.Background(), `INSERT INTO post_scratches (id, user_id, prompt_id, subject, body, visibility, revision)
		VALUES (gen_random_uuid(), $1, nullif($2, '')::uuid, nullif($3, ''), $4, 'direct_only', 1)`, userID, promptID, subject, body)
	require.NoError(t, err)
}

// A new post kept in a scratch pad is listed among the drafts as not saved as
// a draft yet, linking to the Write page it is written on; an emptied pad is
// not listed.
func TestControls_NewPostsInScratchPads(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	ctx := context.Background()
	user, c := newLoggedIn(t, app)
	asker := testutil.Must(factory.User(ctx, app.DB))(t)
	prompt := testutil.Must(factory.PostPrompt(ctx, app.DB, asker.ID, user.ID))(t)
	other := testutil.Must(factory.PostPrompt(ctx, app.DB, asker.ID, user.ID))(t)

	insertPad(t, app, user.ID, "", "", "Plain **pad** text")
	insertPad(t, app, user.ID, prompt.ID, "An answer", "")
	insertPad(t, app, user.ID, other.ID, "", "")

	doc := c.Get("/controls/").RequireStatus(http.StatusOK).Doc()

	plain := doc.Find(`.controls-table a[href="/write"]`)
	require.Equal(t, "Plain pad text", plain.Text())
	require.Contains(t, plain.Parent().Text(), "not saved as a draft yet")
	require.NotContains(t, plain.Parent().Text(), "answers a prompt")

	answer := doc.Find(`.controls-table a[href="/write?prompt=` + prompt.ID + `"]`)
	require.Equal(t, "An answer", answer.Text())
	require.Contains(t, answer.Parent().Text(), "answers a prompt")

	require.Zero(t, doc.Find(`.controls-table a[href="/write?prompt=`+other.ID+`"]`).Length(), "an empty pad is not listed")
}
