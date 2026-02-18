package forms

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms/validation"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SignupFormInput struct {
	Email       string `form:"email"`
	Username    string `form:"username"`
	Password    string `form:"password"`
	Attribution string `form:"attribution"`
}

type SignupForm struct {
	*forms.FormBase[SignupFormInput]
	Sender sender.Sender
}

func SignupFormNew(sender sender.Sender) forms.Form {
	var form forms.Form = &SignupForm{
		FormBase: &forms.FormBase[SignupFormInput]{
			Name:         "signup",
			FormTemplate: "form--signup.html",
			Input:        &SignupFormInput{},
		},
		Sender: sender,
	}

	return form
}

func (f *SignupForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	email := strings.TrimSpace(strings.ToLower(f.Input.Email))
	username := strings.TrimSpace(strings.ToLower(f.Input.Username))

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if reason, isOK := validation.EmailOKToSignup(c, db, f.Sender, email); !isOK {
		f.AddError("email", reason)
	}

	if username == "" {
		f.AddError("username", "username is required")
	} else if err := validation.ValidateUsername(username); err != nil {
		f.AddError("username", err.Error())
	} else {
		exists, err := core.Users(core.UserWhere.Username.EQ(username)).Exists(c, db)

		if err != nil {
			log.Printf("Failed to check username [%s] for duplication: %s", username, err.Error())
			f.AddError("username", "internal error")
		} else if exists {
			f.AddError("username", "this username is not available")
		}
	}

	if f.Input.Password == "" {
		f.AddError("password", "password is required")
	} else if err := validation.ValidatePassword(f.Input.Password); err != nil {
		f.AddError("password", err.Error())
	}

	return f.Errors.PassedValidation()
}

func (f *SignupForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
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

	// we're getting the input from the form and not
	if len(attribution) > 100 {
		attribution = attribution[0:100]
	}

	user, err := auth.Signup(c, exec, f.Sender, email, username, f.Input.Password, attribution)

	if err != nil {
		return nil, err
	}

	if err := mail.ConfirmSignup(c, exec, f.Sender, user); err != nil {
		panic(err)
	}

	return func(c *gin.Context, f forms.Form) {
		c.HTML(http.StatusOK, "partial--signup-goto-email.html", map[string]any{})
	}, nil
}
