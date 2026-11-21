package forms

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
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

// panicOnFatal panics on the failures the forms always panicked on (see accounts.FatalError).
func panicOnFatal(err error) error {
	if _, ok := errors.AsType[*accounts.FatalError](err); ok {
		panic(err)
	}

	return err
}
