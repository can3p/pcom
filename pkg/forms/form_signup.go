package forms

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type SignupFormInput struct {
	Email       string `form:"email"`
	Username    string `form:"username"`
	Attribution string `form:"attribution"`
}

type SignupForm struct {
	*forms.FormBase[SignupFormInput]
	Accounts *accounts.Service
}

func SignupFormNew(accounts *accounts.Service) forms.Form {
	var form forms.Form = &SignupForm{
		FormBase: &forms.FormBase[SignupFormInput]{
			Name:         "signup",
			FormTemplate: "form--signup.html",
			Input:        &SignupFormInput{},
		},
		Accounts: accounts,
	}

	return form
}

func (f *SignupForm) Validate(c *gin.Context) error {
	email := strings.TrimSpace(strings.ToLower(f.Input.Email))
	username := strings.TrimSpace(strings.ToLower(f.Input.Username))

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if err := fieldError(f, "email", f.Accounts.CheckSignupEmail(c, email)); err != nil {
		return err
	}

	if username == "" {
		f.AddError("username", "username is required")
	} else if err := validation.ValidateUsername(username); err != nil {
		f.AddError("username", err.Error())
	} else {
		exists, err := f.Accounts.UsernameTaken(c, username)

		if err != nil {
			log.Printf("Failed to check username [%s] for duplication: %s", username, err.Error())
			f.AddError("username", "internal error")
		} else if exists {
			f.AddError("username", "this username is not available")
		}
	}

	return f.Errors.PassedValidation()
}

func (f *SignupForm) Save(c context.Context) (forms.FormSaveAction, error) {
	email := strings.TrimSpace(strings.ToLower(f.Input.Email))
	username := strings.TrimSpace(strings.ToLower(f.Input.Username))
	attribution := strings.TrimSpace(f.Input.Attribution)

	// we're not enforcing a specific enum of attributions
	// since it's just additional work at the moment
	if !validation.AttributionRE.MatchString(attribution) {
		attribution = "unknown"
	}

	if len(attribution) > 100 {
		attribution = attribution[0:100]
	}

	gc := c.(*gin.Context)

	attemptID, err := f.Accounts.Register(gc.Request.Context(), email, username, attribution)
	if err != nil {
		return nil, panicOnFatal(err)
	}

	return codeStep(gc, attemptID, email)
}

// codeStep makes the attempt the visitor's and answers with the code form for
// it, the same second step logging in has.
func codeStep(c *gin.Context, attemptID, email string) (forms.FormSaveAction, error) {
	if err := auth.SetLoginAttempt(c, attemptID); err != nil {
		return nil, err
	}

	next := &LoginCodeFormInput{Email: email}

	return func(c *gin.Context, _ forms.Form) {
		c.HTML(http.StatusOK, loginCodeTemplate, map[string]any{"Input": next, "Errors": forms.FormErrors{}})
	}, nil
}
