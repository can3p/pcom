package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/feedops"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestAddFeedForm_ValidateEmptyURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("url"))
}

func TestAddFeedForm_ValidateInvalidURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = "not-a-valid-url"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("url"))
}

func TestAddFeedForm_ValidateNoProtocol(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = "example.com/feed.xml"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("url"))
}

func TestAddFeedForm_ValidateValidHTTPURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = "http://example.com/feed.xml"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestAddFeedForm_ValidateValidHTTPSURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = "https://example.com/feed.xml"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestAddFeedForm_SaveFailsOnBlankURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Save doesn't re-run Validate, so calling it directly with a
	// whitespace-only URL reaches feedops.SubscribeToFeed's own error path.
	form := forms.NewAddFeedForm(user)
	form.Input.URL = "   "

	_, err = form.Save(c, db)
	require.Error(t, err)
}

func TestAddFeedForm_SaveTrimsURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/feeds", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.NewAddFeedForm(user)
	form.Input.URL = "  https://example.com/feed.xml  "

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	feeds, err := feedops.GetRssFeeds(ctx, db, user.ID)
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	require.Equal(t, "https://example.com/feed.xml", feeds[0].URL)
}
