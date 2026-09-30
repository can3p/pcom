package forms

import (
	"context"
	"net/http"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
)

type SignupWaitingListFormInput struct {
	Email       string `form:"email"`
	Reason      string `form:"reason"`
	Attribution string `form:"attribution"`
}

type SignupWaitingListForm struct {
	*forms.FormBase[SignupWaitingListFormInput]
	Accounts *accounts.Service
}

func SignupWaitingListFormNew(accounts *accounts.Service) forms.Form {
	var form forms.Form = &SignupWaitingListForm{
		FormBase: &forms.FormBase[SignupWaitingListFormInput]{
			Name:         "signup_waitlist",
			FormTemplate: "form--signup-waitlist.html",
			Input:        &SignupWaitingListFormInput{},
		},
		Accounts: accounts,
	}

	return form
}

func (f *SignupWaitingListForm) Validate(c *gin.Context) error {
	email := pgsession.NormalizeEmail(f.Input.Email)

	if email == "" {
		f.AddError("email", "email is required")
	} else if err := fieldError(f, "email", f.Accounts.CheckWaitingListEmail(c, email)); err != nil {
		return err
	}

	return f.Errors.PassedValidation()
}

func (f *SignupWaitingListForm) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Accounts.JoinWaitingList(c, f.Input.Email, f.Input.Reason, f.Input.Attribution); err != nil {
		return nil, panicOnFatal(err)
	}

	return func(c *gin.Context, f forms.Form) {
		c.HTML(http.StatusOK, "partial--added-to-waitlist.html", map[string]any{})
	}, nil
}
