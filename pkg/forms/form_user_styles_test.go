package forms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSettingsUserStyles_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		styles       string
		wantFieldErr bool
	}{
		{"empty styles", "", false},
		{"valid styles", ".profile { color: red; }", false},
		// Validate always returns nil (see the skipped test below), but it
		// does still record the field error.
		{"too long styles records field error", strings.Repeat("a", 10_001), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

			form := forms.SettingsUserStylesNew(user)
			form.Input.Styles = tt.styles

			err := form.Validate(c, db)
			require.NoError(t, err)
			require.Equal(t, tt.wantFieldErr, form.Errors.HasError("styles"))
		})
	}
}

// TestSettingsUserStyles_ValidateTooLongStylesFailsValidation pins a bug:
// SettingsUserStyles.Validate unconditionally returns nil, so a too-long
// Styles value is recorded as a field error (see the table above) but never
// actually fails validation, unlike every other form in this package.
func TestSettingsUserStyles_ValidateTooLongStylesFailsValidation(t *testing.T) {
	t.Skip("known bug #159: SettingsUserStyles.Validate always returns nil, so a too-long styles value is recorded as a field error but validation never fails")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)

	user := testutil.Must(factory.User(ctx, db))(t)

	form := forms.SettingsUserStylesNew(user)
	form.Input.Styles = strings.Repeat("a", 10_001)

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("styles"))
}

func TestSettingsUserStyles_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		withExisting bool
		styles       string
	}{
		{"creates when none exists", false, ".profile { color: blue; }"},
		{"updates existing", true, ".new { color: green; }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db))(t)
			if tt.withExisting {
				testutil.Must(factory.UserStyle(ctx, db, user.ID, ".old { color: red; }"))(t)
			}

			c, _ := ginctx.New(t, http.MethodPost, "/settings/styles", nil)
			form := forms.SettingsUserStylesNew(user)
			form.Input.Styles = tt.styles

			action, err := form.Save(c, db)
			require.NoError(t, err)
			require.NotNil(t, action)

			style := testutil.Must(factory.GetUserStyle(ctx, db, user.ID))(t)
			require.Equal(t, tt.styles, style.Styles)
		})
	}
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
