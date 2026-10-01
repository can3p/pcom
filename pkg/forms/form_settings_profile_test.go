package forms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSettingsProfile_ValidateAndSave(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db))(t)
	c, _ := ginctx.New(t, http.MethodPost, "/controls/form/save_profile", nil)

	form := forms.SettingsProfileNew(accountsFor(db, nil), user)
	form.Input.About = strings.Repeat("a", 6_001)
	require.Error(t, form.Validate(c))
	require.True(t, form.Errors.HasError("about"))

	form = forms.SettingsProfileNew(accountsFor(db, nil), user)
	form.Input.About = "Hello **there**"
	require.NoError(t, form.Validate(c))

	_, err := form.Save(c)
	require.NoError(t, err)

	settings, err := accountsFor(db, nil).Settings(c, user)
	require.NoError(t, err)
	require.Equal(t, "Hello **there**", settings.ProfileAbout)
}
