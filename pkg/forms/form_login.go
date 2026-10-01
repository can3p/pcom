package forms

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type LoginFormInput struct {
	Email     string `form:"email"`
	ReturnURL string `form:"return_url"`
	Sign      string `form:"sign"`
}

// LoginForm is the first step of logging in: the visitor gives an email, a
// code is mailed to it when it belongs to a user, and the form is replaced by
// the code form. It answers the same for every address.
type LoginForm struct {
	*forms.FormBase[LoginFormInput]
	Accounts *accounts.Service
	// Salt signs return urls.
	Salt string
}

func LoginFormNew(accounts *accounts.Service, salt string) forms.Form {
	var form forms.Form = &LoginForm{
		FormBase: &forms.FormBase[LoginFormInput]{
			Name:         "login",
			FormTemplate: "form--login.html",
			Input:        &LoginFormInput{},
		},
		Accounts: accounts,
		Salt:     salt,
	}

	return form
}

func (f *LoginForm) Validate(c *gin.Context) error {
	f.Input.Email = strings.TrimSpace(f.Input.Email)

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *LoginForm) Save(c context.Context) (forms.FormSaveAction, error) {
	gc := c.(*gin.Context)

	returnURL, sign := f.Input.ReturnURL, f.Input.Sign
	if auth.HashValue(f.Salt, returnURL) != sign {
		returnURL, sign = "", ""
	}

	attemptID, err := f.Accounts.StartLogin(gc.Request.Context(), f.Input.Email, returnURL)
	if err != nil {
		return nil, panicOnFatal(err)
	}

	if err := auth.SetLoginAttempt(gc, attemptID); err != nil {
		return nil, err
	}

	next := &LoginCodeFormInput{Email: f.Input.Email, ReturnURL: returnURL, Sign: sign}

	return func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, loginCodeTemplate, map[string]any{"Input": next, "Errors": forms.FormErrors{}})
	}, nil
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
