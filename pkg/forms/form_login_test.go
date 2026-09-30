package forms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util"
	"github.com/stretchr/testify/require"
)

func TestLoginForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		setup        func(t *testing.T) (email, password string)
		wantErr      bool
		wantErrField string
	}{
		{name: "empty email", setup: func(t *testing.T) (string, string) { return "", "somepassword" },
			wantErr: true, wantErrField: "email"},
		{name: "empty password", setup: func(t *testing.T) (string, string) { return "valid@example.test", "" },
			wantErr: true, wantErrField: "password"},
		{name: "invalid credentials", setup: func(t *testing.T) (string, string) {
			u := testutil.Must(factory.User(ctx, db, factory.WithPassword("correctpassword")))(t)
			return u.Email, "wrongpassword"
		}, wantErr: true},
		{name: "case-insensitive email",
			setup: func(t *testing.T) (string, string) {
				u := testutil.Must(factory.User(ctx, db, factory.WithPassword("correctpassword")))(t)
				return strings.ToUpper(u.Email), "correctpassword"
			}, wantErr: false},
		{name: "valid credentials", setup: func(t *testing.T) (string, string) {
			u := testutil.Must(factory.User(ctx, db, factory.WithPassword("correctpassword")))(t)
			return u.Email, "correctpassword"
		}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
			email, password := tt.setup(t)

			form := forms.LoginFormNew(accountsFor(db, nil)).(*forms.LoginForm)
			form.Input.Email = email
			form.Input.Password = password

			err := form.Validate(c, db)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tt.wantErrField != "" {
				require.True(t, form.Errors.HasError(tt.wantErrField))
			}
		})
	}
}

func TestLoginForm_Save(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		sign         func(returnURL string) string
		wantRedirect func(returnURL string) string
	}{
		{"signed return url redirects there", auth.HashValue, func(u string) string { return util.SiteRoot() + u }},
		{"bad signature redirects home", func(string) string { return "not-a-valid-signature" }, func(string) string { return links.DefaultAuthorizedHome() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db, factory.WithPassword("correctpassword")))(t)
			c, w := ginctx.New(t, http.MethodPost, "/login", nil)

			form := forms.LoginFormNew(accountsFor(db, nil)).(*forms.LoginForm)
			form.Input.Email = user.Email
			form.Input.Password = "correctpassword"
			form.Input.ReturnURL = "/feed"
			form.Input.Sign = tt.sign(form.Input.ReturnURL)

			action, err := form.Save(c, db)
			require.NoError(t, err)
			action(c, form)

			require.Equal(t, tt.wantRedirect(form.Input.ReturnURL), w.Header().Get("HX-Redirect"))
		})
	}
}
