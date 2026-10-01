package forms

import (
	"context"
	"errors"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

const loginCodeTemplate = "form--login-code.html"

// LoginCodeFormInput carries the email and return url only so the form can
// show them and go back to the first step; the attempt lives in the session.
type LoginCodeFormInput struct {
	Code      string `form:"code"`
	Email     string `form:"email"`
	ReturnURL string `form:"return_url"`
	Sign      string `form:"sign"`
}

// LoginCodeForm is the second step of logging in: the code from the mail
// starts the session.
type LoginCodeForm struct {
	*forms.FormBase[LoginCodeFormInput]
	Accounts *accounts.Service
	SiteRoot string
}

func LoginCodeFormNew(accounts *accounts.Service, siteRoot string) forms.Form {
	var form forms.Form = &LoginCodeForm{
		FormBase: &forms.FormBase[LoginCodeFormInput]{
			Name:         "login_code",
			FormTemplate: loginCodeTemplate,
			Input:        &LoginCodeFormInput{},
		},
		Accounts: accounts,
		SiteRoot: siteRoot,
	}

	return form
}

func (f *LoginCodeForm) Validate(c *gin.Context) error {
	f.Input.Code = strings.TrimSpace(f.Input.Code)

	if f.Input.Code == "" {
		f.AddError("code", "code is required")
		return forms.ErrValidationFailed
	}

	return nil
}

func (f *LoginCodeForm) Save(c context.Context) (forms.FormSaveAction, error) {
	gc := c.(*gin.Context)

	attemptID := auth.LoginAttempt(gc)
	if attemptID == "" {
		return f.fail("This login has expired, please start again."), nil
	}

	user, returnURL, err := f.Accounts.FinishLogin(gc.Request.Context(), attemptID, f.Input.Code)

	var invalid *service.ValidationError

	switch {
	case errors.As(err, &invalid):
		f.AddError("code", invalid.Message)

		return forms.FormSaveDefault(true), nil
	case errors.Is(err, service.ErrNotFound):
		return f.fail("This login has expired, please start again."), nil
	case err != nil:
		return nil, panicOnFatal(err)
	}

	// StartSession replaces the whole session, so the attempt goes with it
	if err := auth.StartSession(gc, user); err != nil {
		return nil, err
	}

	if returnURL != "" {
		return forms.FormSaveRedirect(f.SiteRoot + returnURL), nil
	}

	return forms.FormSaveRedirect(links.DefaultAuthorizedHome()), nil
}

func (f *LoginCodeForm) fail(message string) forms.FormSaveAction {
	f.SetFormError(message)

	return forms.FormSaveDefault(true)
}
