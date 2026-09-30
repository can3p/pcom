package forms

import (
	"context"
	"errors"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type LoginFormInput struct {
	Email     string `form:"email"`
	Password  string `form:"password"`
	ReturnURL string `form:"return_url"`
	Sign      string `form:"sign"`
}

type LoginForm struct {
	*forms.FormBase[LoginFormInput]
	Accounts *accounts.Service
	// Salt signs return urls; SiteRoot is what a verified one is appended to.
	Salt     string
	SiteRoot string
}

func LoginFormNew(accounts *accounts.Service, salt, siteRoot string) forms.Form {
	var form forms.Form = &LoginForm{
		FormBase: &forms.FormBase[LoginFormInput]{
			Name:         "login",
			FormTemplate: "form--login.html",
			Input:        &LoginFormInput{},
		},
		Accounts: accounts,
		Salt:     salt,
		SiteRoot: siteRoot,
	}

	return form
}

func (f *LoginForm) Validate(c *gin.Context) error {
	if f.Input.Email == "" {
		f.AddError("email", "email is required")
		return forms.ErrValidationFailed
	}

	if f.Input.Password == "" {
		f.AddError("password", "password is required")
		return forms.ErrValidationFailed
	}

	return f.Accounts.CheckCredentials(c, f.Input.Email, f.Input.Password)
}

func (f *LoginForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := auth.Login(c.(*gin.Context), f.Accounts, f.Input.Email, f.Input.Password); err != nil {
		return nil, err
	}

	if f.Input.ReturnURL != "" && auth.HashValue(f.Salt, f.Input.ReturnURL) == f.Input.Sign {
		return forms.FormSaveRedirect(f.SiteRoot + f.Input.ReturnURL), nil
	}

	return forms.FormSaveRedirect(links.DefaultAuthorizedHome()), nil
}

// fieldError shows a service's ValidationError as an error of the form field
// and returns any other error, which is not the user's to fix.
func fieldError(f interface{ AddError(field, message string) }, field string, err error) error {
	if err == nil {
		return nil
	}

	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		f.AddError(field, invalid.Message)

		return nil
	}

	// anything else is a failing database, which is a bug, not the user's to fix
	panic(err)
}

// panicOnFatal panics on the failures the forms always panicked on (see accounts.FatalError).
func panicOnFatal(err error) error {
	var fatal *accounts.FatalError
	if errors.As(err, &fatal) {
		panic(err)
	}

	return err
}
