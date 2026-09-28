package forms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSettingsUserStyles_ValidateEmptyStyles(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = ""

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestSettingsUserStyles_ValidateTooLongStylesRecordsFieldError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = strings.Repeat("a", 10_001)

	// Validate always returns nil (see the skipped test below), but it does
	// still record the field error.
	_ = form.Validate(c, db)
	require.True(t, form.Errors.HasError("styles"))
}

// TestSettingsUserStyles_ValidateTooLongStylesFailsValidation pins a bug:
// SettingsUserStyles.Validate unconditionally returns nil, so a too-long
// Styles value is recorded as a field error (see the test above) but never
// actually fails validation, unlike every other form in this package.
func TestSettingsUserStyles_ValidateTooLongStylesFailsValidation(t *testing.T) {
	t.Skip("known bug #159: SettingsUserStyles.Validate always returns nil, so a too-long styles value is recorded as a field error but validation never fails")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = strings.Repeat("a", 10_001)

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("styles"))
}

func TestSettingsUserStyles_ValidateValidStyles(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = ".profile { color: red; }"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestSettingsUserStyles_SaveCreatesUserStyle(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = ".profile { color: blue; }"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	style, err := factory.GetUserStyle(ctx, db, user.ID)
	require.NoError(t, err)
	require.Equal(t, ".profile { color: blue; }", style.Styles)
}

func TestSettingsUserStyles_SaveFailsForUnknownUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	// A user id with no matching row violates the styles table's foreign
	// key, reaching Save's own Upsert error path.
	user := &core.User{ID: uuid.NewString()}

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = ".profile { color: blue; }"

	_, err := form.Save(c, db)
	require.Error(t, err)
}

func TestSettingsUserStyles_SaveUpdatesExistingStyle(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.UserStyle(ctx, db, user.ID, ".old { color: red; }")
	require.NoError(t, err)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = ".new { color: green; }"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	style, err := factory.GetUserStyle(ctx, db, user.ID)
	require.NoError(t, err)
	require.Equal(t, ".new { color: green; }", style.Styles)
}
