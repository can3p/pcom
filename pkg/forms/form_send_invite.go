package forms

import (
	"context"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

type SendInviteFormInput struct {
	Email string `form:"email"`
}

type SendInviteForm struct {
	*forms.FormBase[SendInviteFormInput]
	Accounts *accounts.Service
	User     *core.User
}

func SendInviteFormNew(accounts *accounts.Service, u *core.User) forms.Form {
	var form forms.Form = &SendInviteForm{
		FormBase: &forms.FormBase[SendInviteFormInput]{
			Name:         "send_invite",
			FormTemplate: "form--send-invite.html",
			Input:        &SendInviteFormInput{},
			ExtraTemplateData: map[string]any{
				"User": u,
			},
		},
		Accounts: accounts,
		User:     u,
	}

	return form
}

func (f *SendInviteForm) Validate(c *gin.Context, db boil.ContextExecutor) error {
	if f.Input.Email == "" {
		f.AddError("email", "email is required")
	} else if err := fieldError(f, "email", f.Accounts.CheckInviteEmail(c, f.Input.Email)); err != nil {
		return err
	}

	return f.Errors.PassedValidation()
}

func (f *SendInviteForm) Save(c context.Context, exec boil.ContextExecutor) (forms.FormSaveAction, error) {
	if err := f.Accounts.SendInvite(c, f.User, f.Input.Email); err != nil {
		return nil, err
	}

	return forms.FormSaveFullReload, nil
}
