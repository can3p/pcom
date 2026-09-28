package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestWhitelistConnection_ValidateEmptyUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user).(*forms.WhitelistConnection)
	form.Input.Username = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestWhitelistConnection_ValidateOwnUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user).(*forms.WhitelistConnection)
	form.Input.Username = user.Username

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestWhitelistConnection_ValidateNonexistentUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user).(*forms.WhitelistConnection)
	form.Input.Username = "nonexistent"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestWhitelistConnection_ValidateExistingConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user1, err := factory.User(ctx, db)
	require.NoError(t, err)

	user2, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, user1.ID, user2.ID)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user1).(*forms.WhitelistConnection)
	form.Input.Username = user2.Username

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestWhitelistConnection_ValidateAlreadyWhitelisted(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user1, err := factory.User(ctx, db)
	require.NoError(t, err)

	user2, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Whitelist(ctx, db, user1.ID, user2.ID)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user1).(*forms.WhitelistConnection)
	form.Input.Username = user2.Username

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestWhitelistConnection_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user1, err := factory.User(ctx, db)
	require.NoError(t, err)

	user2, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user1).(*forms.WhitelistConnection)
	form.Input.Username = user2.Username

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestWhitelistConnection_SaveFailsForNonexistentUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Save doesn't re-run Validate, so calling it directly with a username
	// that doesn't exist reaches the target lookup's own error path.
	form := forms.WhitelistConnectionNew(user).(*forms.WhitelistConnection)
	form.Input.Username = "nonexistent"

	_, err = form.Save(c, db)
	require.Error(t, err)
}

func TestWhitelistConnection_SaveCreatesWhitelist(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

	user1, err := factory.User(ctx, db)
	require.NoError(t, err)

	user2, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.WhitelistConnectionNew(user1).(*forms.WhitelistConnection)
	form.Input.Username = user2.Username

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	exists, err := factory.WhitelistExists(ctx, db, user1.ID, user2.ID)
	require.NoError(t, err)
	require.True(t, exists)
}
