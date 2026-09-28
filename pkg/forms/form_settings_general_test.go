package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSettingsGeneralForm_ValidateEmptyTimezone(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = ""
	form.Input.ProfileVisibility = string(core.ProfileVisibilityPublic)

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("timezone"))
}

func TestSettingsGeneralForm_ValidateInvalidTimezone(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "Invalid/Timezone"
	form.Input.ProfileVisibility = string(core.ProfileVisibilityPublic)

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("timezone"))
}

func TestSettingsGeneralForm_ValidateEmptyProfileVisibility(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "America/New_York"
	form.Input.ProfileVisibility = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("profile_visibility"))
}

func TestSettingsGeneralForm_ValidateInvalidProfileVisibility(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "America/New_York"
	form.Input.ProfileVisibility = "invalid"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("profile_visibility"))
}

func TestSettingsGeneralForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "America/New_York"
	form.Input.ProfileVisibility = string(core.ProfileVisibilityConnections)

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestSettingsGeneralForm_SaveUpdatesSettings(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "America/Los_Angeles"
	form.Input.ProfileVisibility = string(core.ProfileVisibilityRegisteredUsers)

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	updatedUser, err := factory.GetUser(ctx, db, user.ID)
	require.NoError(t, err)
	require.Equal(t, "America/Los_Angeles", updatedUser.Timezone)
	require.Equal(t, core.ProfileVisibilityRegisteredUsers, updatedUser.ProfileVisibility)
}
