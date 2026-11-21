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
