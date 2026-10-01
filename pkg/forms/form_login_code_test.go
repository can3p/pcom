package forms_test

import (
	"context"
	gogoforms "github.com/can3p/gogo/forms"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

func TestLoginCodeForm_ValidateNeedsACode(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodPost, "/login/code", nil)
	form := forms.LoginCodeFormNew(nil, testSiteRoot).(*forms.LoginCodeForm)

	require.ErrorIs(t, form.Validate(c), gogoforms.ErrValidationFailed)
	require.True(t, form.Errors.HasError("code"))
}

func TestLoginCodeForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := codeAccounts(db)

	// attempt returns a context whose session holds a fresh attempt of a user
	// and the code that logs it in.
	attempt := func(returnURL string) (*forms.LoginCodeForm, string, func() string) {
		user := testutil.Must(factory.User(ctx, db))(t)
		id := testutil.Must(svc.StartLogin(ctx, user.Email, returnURL))(t)
		code := testutil.Must(svc.IssueLoginCode(ctx, id))(t)
		c, w := ginctx.New(t, http.MethodPost, "/login/code", nil)
		require.NoError(t, auth.SetLoginAttempt(c, id))

		form := forms.LoginCodeFormNew(svc, testSiteRoot).(*forms.LoginCodeForm)
		form.Input.Code = code

		return form, user.ID, func() string {
			action, err := form.Save(c)
			require.NoError(t, err)
			require.NotNil(t, action)

			if form.Errors.HasError("code") || form.FormError != "" {
				return ""
			}

			require.Equal(t, user.ID, sessions.Default(c).Get("user"))
			action(c, form)

			return w.Header().Get("HX-Redirect")
		}
	}

	t.Run("the code starts the session and goes home", func(t *testing.T) {
		_, _, save := attempt("")
		require.Equal(t, links.DefaultAuthorizedHome(), save())
	})

	t.Run("the code goes to the return url of the attempt", func(t *testing.T) {
		_, _, save := attempt("/write")
		require.Equal(t, testSiteRoot+"/write", save())
	})

	t.Run("a wrong code is an error on the field", func(t *testing.T) {
		form, _, save := attempt("")
		form.Input.Code = "000000"
		require.Empty(t, save())
		require.True(t, form.Errors.HasError("code"))
	})

	t.Run("no attempt in the session is an expired login", func(t *testing.T) {
		c, _ := ginctx.New(t, http.MethodPost, "/login/code", nil)
		form := forms.LoginCodeFormNew(svc, testSiteRoot).(*forms.LoginCodeForm)
		form.Input.Code = "123456"

		_, err := form.Save(c)
		require.NoError(t, err)
		require.NotEmpty(t, form.FormError)
	})
}
