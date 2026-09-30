package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/connections"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestWhitelistConnection_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		username     func(t *testing.T, user1 *core.User) string
		wantErrField string
	}{
		{"empty username", func(t *testing.T, user1 *core.User) string { return "" }, "username"},
		{"own username", func(t *testing.T, user1 *core.User) string { return user1.Username }, "username"},
		{"nonexistent user", func(t *testing.T, user1 *core.User) string { return "nonexistent" }, "username"},
		{"existing connection", func(t *testing.T, user1 *core.User) string {
			user2 := testutil.Must(factory.User(ctx, db))(t)
			_, _, err := factory.Connect(ctx, db, user1.ID, user2.ID)
			require.NoError(t, err)
			return user2.Username
		}, "username"},
		{"already whitelisted", func(t *testing.T, user1 *core.User) string {
			user2 := testutil.Must(factory.User(ctx, db))(t)
			testutil.Must(factory.Whitelist(ctx, db, user1.ID, user2.ID))(t)
			return user2.Username
		}, "username"},
		{"success", func(t *testing.T, user1 *core.User) string {
			return testutil.Must(factory.User(ctx, db))(t).Username
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user1 := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

			form := forms.WhitelistConnectionNew(user1, connections.New(repo.New(db))).(*forms.WhitelistConnection)
			form.Input.Username = tt.username(t, user1)

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

func TestWhitelistConnection_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (username, targetID string)
		wantErr bool
	}{
		{"fails for nonexistent user", func(t *testing.T) (string, string) {
			// Save doesn't re-run Validate, so calling it directly with a
			// username that doesn't exist reaches the target lookup's own
			// error path.
			return "nonexistent", ""
		}, true},
		{"creates whitelist", func(t *testing.T) (string, string) {
			user2 := testutil.Must(factory.User(ctx, db))(t)
			return user2.Username, user2.ID
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user1 := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/whitelist", nil)

			username, targetID := tt.setup(t)
			form := forms.WhitelistConnectionNew(user1, connections.New(repo.New(db))).(*forms.WhitelistConnection)
			form.Input.Username = username

			action, err := form.Save(c, db)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, action)

			exists := testutil.Must(factory.WhitelistExists(ctx, db, user1.ID, targetID))(t)
			require.True(t, exists)
		})
	}
}
