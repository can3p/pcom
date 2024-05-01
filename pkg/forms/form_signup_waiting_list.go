package forms

import (
	"context"
	"net/http"
	"strings"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/admin"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SignupWaitingListFormInput struct {
	Email       string `form:"email"`
	Reason      string `form:"reason"`
	Attribution string `form:"attribution"`
}

type SignupWaitingListForm struct {
	*forms.FormBase[SignupWaitingListFormInput]
	Sender sender.Sender
}

func SignupWaitingListFormNew(sender sender.Sender) forms.Form {
	var form forms.Form = &SignupWaitingListForm{
		FormBase: &forms.FormBase[SignupWaitingListFormInput]{
			Name:         "signup_waitlist",
			FormTemplate: "form--signup-waitlist.html",
			Input:        &SignupWaitingListFormInput{},
		},
		Sender: sender,
	}

	return form
}

func (f *SignupWaitingListForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	email := strings.TrimSpace(strings.ToLower(f.Input.Email))

	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if reason, isOK := web.EmailOKToAddToWaitingList(c, db, email); !isOK {
		f.AddError("email", reason)
	}

	return f.Errors.PassedValidation()
}

func (f *SignupWaitingListForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	request := core.UserSignupRequest{
		ID:                uuid.NewString(),
		Email:             f.Input.Email,
		Reason:            null.NewString(f.Input.Reason, f.Input.Reason != ""),
		SignupAttribution: null.NewString(f.Input.Attribution, f.Input.Attribution != ""),
	}

	request.UpsertP(c, exec, false, []string{core.UserSignupRequestColumns.Email}, boil.Infer(), boil.Infer())

	if err := mail.ConfirmWaitingList(c, exec, f.Sender, &request); err != nil {
		panic(err)
	}

	go admin.NotifyNewWaitingListMember(c, f.Sender, &request)

	return func(c *gin.Context, f forms.Form) {
		c.HTML(http.StatusOK, "partial--added-to-waitlist.html", map[string]interface{}{})
	}, nil
}
