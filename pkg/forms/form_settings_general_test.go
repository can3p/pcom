package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model"
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
		subscribe    bool
		wantErrField string
	}{
		{"empty timezone", "", string(model.ProfileVisibilityPublic), false, "timezone"},
		{"invalid timezone", "Invalid/Timezone", string(model.ProfileVisibilityPublic), false, "timezone"},
		{"empty profile visibility", "America/New_York", "", false, "profile_visibility"},
		{"invalid profile visibility", "America/New_York", "invalid", false, "profile_visibility"},
		{"success", "America/New_York", string(model.ProfileVisibilityConnections), false, ""},
		{"subscriptions on a public journal", "America/New_York", string(model.ProfileVisibilityPublic), true, ""},
		{"subscriptions on a journal for connections", "America/New_York", string(model.ProfileVisibilityConnections), true, "email_subscriptions_enabled"},
		{"subscriptions on a journal for accounts", "America/New_York", string(model.ProfileVisibilityRegisteredUsers), true, "email_subscriptions_enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/general", nil)

			form := forms.SettingsGeneralFormNew(accountsFor(db, nil), user, 0)
			form.Input.Timezone = tt.timezone
			form.Input.ProfileVisibility = tt.visibility
			form.Input.EmailSubscriptions = tt.subscribe

			err := form.Validate(c)
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

	form := forms.SettingsGeneralFormNew(accountsFor(db, nil), user, 0)
	form.Input.Timezone = "America/Los_Angeles"
	form.Input.ProfileVisibility = string(model.ProfileVisibilityPublic)
	form.Input.EmailSubscriptions = true

	action, err := form.Save(c)
	require.NoError(t, err)
	require.NotNil(t, action)

	updatedUser := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.Equal(t, "America/Los_Angeles", updatedUser.Timezone)
	require.Equal(t, model.ProfileVisibilityPublic, updatedUser.ProfileVisibility)
	require.True(t, updatedUser.EmailSubscriptionsEnabled)
}
