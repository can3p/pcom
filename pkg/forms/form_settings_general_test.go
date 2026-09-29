package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSettingsGeneralForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		timezone     string
		visibility   string
		wantErrField string
	}{
		{"empty timezone", "", string(core.ProfileVisibilityPublic), "timezone"},
		{"invalid timezone", "Invalid/Timezone", string(core.ProfileVisibilityPublic), "timezone"},
		{"empty profile visibility", "America/New_York", "", "profile_visibility"},
		{"invalid profile visibility", "America/New_York", "invalid", "profile_visibility"},
		{"success", "America/New_York", string(core.ProfileVisibilityConnections), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

			form := forms.SettingsGeneralFormNew(user)
			form.Input.Timezone = tt.timezone
			form.Input.ProfileVisibility = tt.visibility

			err := form.Validate(c, db)
			if tt.wantErrField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, form.Errors.HasError(tt.wantErrField))
		})
	}
}

func TestSettingsGeneralForm_SaveUpdatesSettings(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

	user := testutil.Must(factory.User(ctx, db))(t)

	form := forms.SettingsGeneralFormNew(user)
	form.Input.Timezone = "America/Los_Angeles"
	form.Input.ProfileVisibility = string(core.ProfileVisibilityRegisteredUsers)

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	updatedUser := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.Equal(t, "America/Los_Angeles", updatedUser.Timezone)
	require.Equal(t, core.ProfileVisibilityRegisteredUsers, updatedUser.ProfileVisibility)
}
